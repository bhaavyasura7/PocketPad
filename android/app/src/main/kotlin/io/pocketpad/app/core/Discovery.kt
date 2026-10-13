package io.pocketpad.app.core

import io.pocketpad.protocol.Codec
import io.pocketpad.protocol.ProtocolException
import io.pocketpad.protocol.Wire
import java.net.DatagramPacket
import java.net.DatagramSocket
import java.net.InetAddress
import java.net.InetSocketAddress

/**
 * Broadcasts DISCOVER and collects DISCOVER_REPLY for a short window. The
 * reply only fills in an address; the handshake proves the receiver knows
 * the pairing secret. Runs on the caller's (background) thread.
 */
object Discovery {

    data class Found(val name: String, val host: String, val port: Int, val caps: Int)

    fun scan(timeoutMs: Long = 1000): List<Found> {
        val socket = DatagramSocket()
        try {
            socket.broadcast = true
            val probe = ByteArray(Wire.DISCOVER_SIZE)
            Codec.putDiscover(probe)
            val target = InetSocketAddress(InetAddress.getByName("255.255.255.255"), Wire.DEFAULT_PORT)
            socket.send(DatagramPacket(probe, probe.size, target))

            socket.soTimeout = 250
            val buf = ByteArray(Wire.MAX_DATAGRAM + 1)
            val found = LinkedHashMap<String, Found>()
            val deadline = System.nanoTime() + timeoutMs * 1_000_000
            while (System.nanoTime() < deadline) {
                val pkt = DatagramPacket(buf, buf.size)
                val n = try {
                    socket.receive(pkt)
                    pkt.length
                } catch (_: Exception) {
                    continue
                }
                val type = try {
                    Codec.checkHeader(buf, n)
                } catch (_: ProtocolException) {
                    continue
                }
                if (type != Wire.TYPE_DISCOVER_REPLY) continue
                val reply = try {
                    Codec.decodeDiscoverReply(buf, n)
                } catch (_: ProtocolException) {
                    continue
                }
                val host = pkt.address?.hostAddress ?: continue
                val f = Found(reply.name, host, reply.port, reply.caps)
                found["$host:${reply.port}"] = f
            }
            return found.values.toList()
        } catch (_: Exception) {
            return emptyList()
        } finally {
            socket.close()
        }
    }
}
