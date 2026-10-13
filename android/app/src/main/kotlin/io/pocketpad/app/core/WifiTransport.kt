package io.pocketpad.app.core

import android.os.SystemClock
import io.pocketpad.protocol.Codec
import io.pocketpad.protocol.ControllerState
import io.pocketpad.protocol.Handshake
import io.pocketpad.protocol.Opener
import io.pocketpad.protocol.ProtocolException
import io.pocketpad.protocol.Sealer
import io.pocketpad.protocol.Wire
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import java.net.DatagramPacket
import java.net.DatagramSocket
import java.net.InetAddress
import java.net.SocketTimeoutException
import java.util.concurrent.TimeUnit
import java.util.concurrent.locks.ReentrantLock
import kotlin.concurrent.withLock
import kotlin.math.max

/**
 * The phone side of the Wi-Fi protocol: handshake, one sender thread and one
 * receiver thread, exactly mirroring the Go reference client
 * (desktop/internal/client/client.go). Reconnects with backoff when the
 * session is lost; only a REJECT or silence is terminal.
 */
class WifiTransport(
    private val host: String,
    private val port: Int,
    private val pairKey: ByteArray,
    private val deviceId: Long,
    private val deviceName: String,
    private val requestedRateHz: Int,
    private val input: InputState,
    private val rumbler: Rumbler,
    private val battery: () -> BatteryReader.Reading,
) : Transport {

    private val _status = MutableStateFlow<TransportStatus>(TransportStatus.Idle)
    override val status: StateFlow<TransportStatus> = _status.asStateFlow()
    private val _stats = MutableStateFlow(Stats())
    override val stats: StateFlow<Stats> = _stats.asStateFlow()

    @Volatile private var running = false
    private var thread: Thread? = null

    // Sender wake-up: the input listener signals, the sender awaits.
    private val senderLock = ReentrantLock()
    private val senderWake = senderLock.newCondition()
    private val wakeListener: () -> Unit = { senderLock.withLock { senderWake.signalAll() } }

    // The Sealer is shared by the sender and the goodbye path; the lock only
    // covers building one packet (microseconds), never the socket write.
    private val sealLock = Any()

    @Volatile private var echoUs = 0
    @Volatile private var rttUs = 0

    private val neutral = ControllerState()

    override fun start() {
        if (running) return
        running = true
        input.onChanged = wakeListener
        thread = Thread(::runSession, "pp-wifi").apply {
            isDaemon = true
            start()
        }
    }

    override fun stop() {
        if (!running) return
        running = false
        input.onChanged = null
        senderLock.withLock { senderWake.signalAll() }
        thread?.join(2500)
        thread = null
        _status.value = TransportStatus.Idle
        _stats.value = Stats()
    }

    private class Session(val sealer: Sealer, val opener: Opener, val welcome: Codec.Welcome)

    private class RejectException(val reason: Int) : Exception()
    private class NoAnswerException : Exception()

    private fun runSession() {
        var backoffMs = 500L
        while (running) {
            val addr = try {
                InetAddress.getByName(host)
            } catch (e: Exception) {
                _status.value = TransportStatus.Error(ErrorKind.SOCKET, e.message ?: e.javaClass.simpleName)
                running = false
                return
            }
            val sock = DatagramSocket()
            try {
                sock.connect(addr, port)
                _status.value = TransportStatus.Connecting(0, 4)

                val session = try {
                    handshake(sock)
                } catch (_: InterruptedException) {
                    return
                } catch (e: RejectException) {
                    _status.value = TransportStatus.Error(ErrorKind.REJECT, reason = e.reason)
                    running = false
                    return
                } catch (_: NoAnswerException) {
                    _status.value = TransportStatus.Error(ErrorKind.NO_ANSWER)
                    running = false
                    return
                }

                backoffMs = 500
                val rate = requestedRateHz.coerceIn(60, session.welcome.maxRateHz.coerceAtLeast(60))
                _status.value = TransportStatus.Connected(rate, host)
                _stats.value = Stats()

                val sender = Thread({ senderLoop(sock, session, rate) }, "pp-sender").apply {
                    isDaemon = true
                    start()
                }
                readLoop(sock, session)

                // Wake the sender so it can finish, then wait for it.
                senderLock.withLock { senderWake.signalAll() }
                sender.join(1000)
                if (!running) {
                    // Leaving the controller screen: neutral state, then DISCONNECT.
                    sayGoodbye(sock, session)
                }
            } catch (_: Exception) {
                if (!running) return
                // Transient (network change, ICMP, etc.): reconnect below.
            } finally {
                sock.close()
            }
            if (!running) return
            _status.value = TransportStatus.Reconnecting(backoffMs)
            try {
                Thread.sleep(backoffMs)
            } catch (_: InterruptedException) {
                return
            }
            backoffMs = (backoffMs * 2).coerceAtMost(5000)
        }
    }

    private fun handshake(sock: DatagramSocket): Session {
        val pkt = ByteArray(Wire.HELLO_SIZE + Wire.HANDSHAKE_TAG_SIZE)
        val inBuf = ByteArray(Wire.MAX_DATAGRAM + 1)
        val empty = ByteArray(0)

        for (attempt in 0 until 4) {
            if (!running) throw InterruptedException()
            // A fresh nonce every attempt: the receiver drops repeats.
            val nonce = Handshake.nonce()
            Codec.putHello(pkt, deviceId, nonce, CAPABILITIES, deviceName)
            System.arraycopy(
                Handshake.tag(pairKey, pkt, Wire.HELLO_SIZE, empty), 0,
                pkt, Wire.HELLO_SIZE, Wire.HANDSHAKE_TAG_SIZE,
            )
            _status.value = TransportStatus.Connecting(attempt + 1, 4)
            sock.send(DatagramPacket(pkt, pkt.size))

            val deadline = SystemClock.elapsedRealtime() + (250L shl attempt)
            while (SystemClock.elapsedRealtime() < deadline) {
                if (!running) throw InterruptedException()
                val remaining = deadline - SystemClock.elapsedRealtime()
                sock.soTimeout = remaining.toInt().coerceIn(1, 2500)
                val inPkt = DatagramPacket(inBuf, inBuf.size)
                val n = try {
                    sock.receive(inPkt)
                    inPkt.length
                } catch (_: SocketTimeoutException) {
                    break // next attempt
                }
                // ParseHeader, not CheckHeader: a REJECT may carry a foreign
                // version and must still be understood.
                val type = try {
                    Codec.parseHeader(inBuf, n)
                } catch (_: ProtocolException) {
                    continue
                }
                if (type == Wire.TYPE_REJECT && n == Wire.REJECT_SIZE) {
                    throw RejectException(Codec.decodeReject(inBuf, n).reason)
                }
                val checked = try {
                    Codec.checkHeader(inBuf, n)
                } catch (_: ProtocolException) {
                    continue
                }
                if (checked != Wire.TYPE_WELCOME) continue
                if (!Handshake.verify(pairKey, inBuf, n, nonce)) continue
                val w = Codec.decodeWelcome(inBuf)
                val keys = Handshake.deriveSessionKeys(pairKey, deviceId, nonce, w.serverNonce, w.sessionId)
                return Session(Sealer(keys.clientToServer), Opener(keys.serverToClient), w)
            }
        }
        throw NoAnswerException()
    }

    private fun readLoop(sock: DatagramSocket, session: Session) {
        val buf = ByteArray(Wire.MAX_DATAGRAM + 1)
        var lastRecv = SystemClock.elapsedRealtime()
        sock.soTimeout = 250
        while (running) {
            val pkt = DatagramPacket(buf, buf.size)
            val n = try {
                sock.receive(pkt)
                pkt.length
            } catch (_: SocketTimeoutException) {
                // Session timeout: no authenticated packet for 3 s.
                if (SystemClock.elapsedRealtime() - lastRecv > 3000) return
                continue
            } catch (_: Exception) {
                return
            }
            val type = try {
                Codec.checkHeader(buf, n)
            } catch (_: ProtocolException) {
                continue
            }
            if (!Wire.isSessionType(type)) continue
            try {
                session.opener.open(buf, n)
            } catch (_: ProtocolException) {
                continue
            }
            lastRecv = SystemClock.elapsedRealtime()
            when (type) {
                Wire.TYPE_HEARTBEAT -> try {
                    Codec.checkHeartbeat(buf)
                    echoUs = Codec.heartbeatTimeUs(buf)
                    val echo = Codec.heartbeatEchoUs(buf)
                    if (echo != 0) {
                        rttUs = max(nowUs() - echo, 1)
                        _stats.value = _stats.value.copy(rttUs = rttUs)
                    }
                } catch (_: ProtocolException) {
                }
                Wire.TYPE_RUMBLE -> try {
                    val d = Codec.rumbleDurationMs(buf)
                    rumbler.onRumble(Codec.rumbleLeft(buf), Codec.rumbleRight(buf), d)
                } catch (_: ProtocolException) {
                }
                Wire.TYPE_DISCONNECT -> return
            }
        }
    }

    private fun senderLoop(sock: DatagramSocket, session: Session, rateHz: Int) {
        val schedule = SendSchedule(rateHz, session.welcome.heartbeatMs)
        val state = ControllerState()
        val lastSent = ControllerState()
        val buf = ByteArray(Wire.MAX_DATAGRAM)
        var sent = 0
        var lastCount = 0
        var lastCountMs = SystemClock.elapsedRealtime()

        while (running) {
            val now = monotonicUs()
            input.snapshot(state)
            val changed = !state.sameOnWire(lastSent)
            val held = !state.isNeutral
            val action = schedule.poll(now, changed, held)

            try {
                if (action and SendSchedule.STATE != 0) {
                    val flags = if (action and SendSchedule.SNAPSHOT != 0) Wire.FLAG_SNAPSHOT else 0
                    val len = synchronized(sealLock) {
                        val ext = session.sealer.next()
                        Codec.putState(buf, (ext and 0xFFFF).toInt(), state, flags)
                        session.sealer.seal(buf, Wire.STATE_SIZE, ext)
                    }
                    sock.send(DatagramPacket(buf, len))
                    lastSent.copyFrom(state)
                    schedule.sentState(now)
                    sent++
                }
                if (action and SendSchedule.HEARTBEAT != 0) {
                    val b = battery()
                    val t = nowUs() // wire timestamp (u32, wraps)
                    val len = synchronized(sealLock) {
                        val ext = session.sealer.next()
                        Codec.putHeartbeat(
                            buf, (ext and 0xFFFF).toInt(), t, echoUs, rttUs,
                            if (b.percent in 0..100) b.percent else Wire.BATTERY_UNKNOWN,
                            if (b.charging) Wire.HEARTBEAT_CHARGING else 0,
                        )
                        session.sealer.seal(buf, Wire.HEARTBEAT_SIZE, ext)
                    }
                    sock.send(DatagramPacket(buf, len))
                    schedule.sentHeartbeat(now)

                    val nowMs = SystemClock.elapsedRealtime()
                    val elapsed = nowMs - lastCountMs
                    if (elapsed >= 1000) {
                        _stats.value = _stats.value.copy(rateHz = ((sent - lastCount) * 1000L / elapsed).toInt())
                        lastCount = sent
                        lastCountMs = nowMs
                    }
                }
            } catch (_: Exception) {
                return // the read loop notices the same socket failure
            }

            val waitUs =
                schedule.nextDueUs(monotonicUs(), changed && (action and SendSchedule.STATE == 0), !state.isNeutral)
            senderLock.withLock { senderWake.await(waitUs, TimeUnit.MICROSECONDS) }
        }
    }

    /** Best-effort neutral STATE then DISCONNECT(user), on user stop. */
    private fun sayGoodbye(sock: DatagramSocket, session: Session) {
        try {
            val buf = ByteArray(Wire.MAX_DATAGRAM)
            val stateLen = synchronized(sealLock) {
                val ext = session.sealer.next()
                Codec.putState(buf, (ext and 0xFFFF).toInt(), neutral, 0)
                session.sealer.seal(buf, Wire.STATE_SIZE, ext)
            }
            sock.send(DatagramPacket(buf, stateLen))
            val discLen = synchronized(sealLock) {
                val ext = session.sealer.next()
                Codec.putDisconnect(buf, (ext and 0xFFFF).toInt(), Wire.REASON_USER)
                session.sealer.seal(buf, Wire.DISCONNECT_SIZE, ext)
            }
            sock.send(DatagramPacket(buf, discLen))
        } catch (_: Exception) {
        }
    }

    /** Wrapping monotonic clock in microseconds; never 0 (0 means "none"). */
    private fun nowUs(): Int = (SystemClock.elapsedRealtimeNanos() / 1000).toInt() or 1

    /** Monotonic microseconds for local scheduling (never wraps in practice). */
    private fun monotonicUs(): Long = SystemClock.elapsedRealtimeNanos() / 1000

    companion object {
        private const val CAPABILITIES =
            Wire.CAP_RUMBLE or Wire.CAP_BATTERY or Wire.CAP_GUIDE or Wire.CAP_LATENCY_ECHO
    }
}
