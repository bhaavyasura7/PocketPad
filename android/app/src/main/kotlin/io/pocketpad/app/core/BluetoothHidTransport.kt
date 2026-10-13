package io.pocketpad.app.core

import android.bluetooth.BluetoothAdapter
import android.bluetooth.BluetoothDevice
import android.bluetooth.BluetoothHidDevice
import android.bluetooth.BluetoothHidDeviceAppSdpSettings
import android.bluetooth.BluetoothManager
import android.bluetooth.BluetoothProfile
import android.content.Context
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import io.pocketpad.protocol.ControllerState
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import java.util.concurrent.ExecutorService
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import java.util.concurrent.locks.ReentrantLock
import kotlin.concurrent.withLock

/**
 * Bluetooth HID controller mode (docs/DEVELOPMENT.md, "Bluetooth HID
 * report"): the phone registers as a HID gamepad and sends 13-byte reports
 * on every change, capped at
 * 125 Hz, with a 100 ms resend while anything is held. Rumble is not
 * available over a generic HID pad, so it is left out.
 */
class BluetoothHidTransport(
    private val context: Context,
    private val input: InputState,
) : Transport {

    private val _status = MutableStateFlow<TransportStatus>(TransportStatus.Idle)
    override val status: StateFlow<TransportStatus> = _status.asStateFlow()
    private val _stats = MutableStateFlow(Stats(rateHz = MAX_RATE_HZ))
    override val stats: StateFlow<Stats> = _stats.asStateFlow()

    private val adapter: BluetoothAdapter? =
        (context.getSystemService(Context.BLUETOOTH_SERVICE) as? BluetoothManager)?.adapter

    @Volatile private var running = false

    @Volatile private var connected = false

    @Volatile private var registered = false

    @Volatile private var hid: BluetoothHidDevice? = null

    @Volatile private var host: BluetoothDevice? = null

    // The host asked for before the profile proxy and registration were
    // ready; connected automatically once they are.
    @Volatile private var pendingHost: BluetoothDevice? = null

    private var sender: Thread? = null

    private val senderLock = ReentrantLock()
    private val senderWake = senderLock.newCondition()
    private val wakeListener: () -> Unit = { senderLock.withLock { senderWake.signalAll() } }

    private val report = ByteArray(HidReport.DATA_SIZE)
    private val lastSent = ControllerState()

    private val mainHandler = Handler(Looper.getMainLooper())

    // Created in start() so the transport can be stopped and started again
    // (a shut-down executor cannot be reused for registerApp).
    private var executor: ExecutorService? = null

    val available: Boolean get() = adapter != null

    val enabled: Boolean
        get() = try {
            adapter?.isEnabled == true
        } catch (_: SecurityException) {
            false
        }

    /** Bonded devices, for the host picker. */
    fun pairedHosts(): List<BluetoothDevice> = try {
        adapter?.bondedDevices?.toList().orEmpty()
    } catch (_: SecurityException) {
        emptyList()
    }

    /** Calls [start] first; registers the HID app and waits for a host. */
    override fun start() {
        if (running) return
        val a = adapter ?: return fail(ErrorKind.BT_UNAVAILABLE)
        if (!a.isEnabled) return fail(ErrorKind.BT_OFF)
        running = true
        executor = Executors.newSingleThreadExecutor { r -> Thread(r, "pp-bt-cb").apply { isDaemon = true } }
        input.onChanged = wakeListener
        _status.value = TransportStatus.Connecting()
        try {
            if (!a.getProfileProxy(context, serviceListener, BluetoothProfile.HID_DEVICE)) {
                fail(ErrorKind.BT_UNAVAILABLE)
            } else {
                // Several phones report the HID device profile as available but
                // ship without the service, so onServiceConnected never fires.
                // Treat a silent timeout as "this phone cannot be a gamepad".
                mainHandler.postDelayed({ if (running && !registered) fail(ErrorKind.BT_UNAVAILABLE) }, 5000)
            }
        } catch (_: SecurityException) {
            fail(ErrorKind.BT_PERMISSION)
        }
    }

    private fun fail(kind: ErrorKind) {
        _status.value = TransportStatus.Error(kind)
        running = false
        input.onChanged = null
    }

    /** Connects to an already bonded host. Safe to call before registration. */
    fun connect(device: BluetoothDevice) {
        host = device
        pendingHost = device
        val h = hid
        if (h != null && registered) {
            doConnect(h, device)
        } else {
            // getProfileProxy and registerApp are asynchronous; onServiceConnected
            // picks the pending host up when they finish.
            _status.value = TransportStatus.Connecting()
        }
    }

    private fun doConnect(h: BluetoothHidDevice, device: BluetoothDevice) {
        try {
            if (!h.connect(device)) {
                _status.value = TransportStatus.Error(ErrorKind.BT_CONNECT, safeName(device))
            }
        } catch (_: SecurityException) {
            _status.value = TransportStatus.Error(ErrorKind.BT_PERMISSION)
        }
    }

    override fun stop() {
        if (!running && hid == null) return
        running = false
        input.onChanged = null

        // Neutral report first, while the host may still be listening.
        val neutral = ControllerState()
        HidReport.encode(neutral, report)
        if (connected) trySend(report)

        connected = false
        senderLock.withLock { senderWake.signalAll() }
        sender?.join(500)
        sender = null

        try {
            hid?.unregisterApp()
        } catch (_: SecurityException) {
        } catch (_: Exception) {
        }
        val h = hid
        hid = null
        registered = false
        pendingHost = null
        try {
            if (h != null) adapter?.closeProfileProxy(BluetoothProfile.HID_DEVICE, h)
        } catch (_: Exception) {
        }
        executor?.shutdown()
        executor = null
        _status.value = TransportStatus.Idle
        _stats.value = Stats()
    }

    private val serviceListener = object : BluetoothProfile.ServiceListener {
        override fun onServiceConnected(profile: Int, proxy: BluetoothProfile) {
            if (profile != BluetoothProfile.HID_DEVICE) return
            if (!running) {
                try {
                    adapter?.closeProfileProxy(profile, proxy)
                } catch (_: Exception) {
                }
                return
            }
            val h = proxy as BluetoothHidDevice
            hid = h
            val exec = executor
            if (exec == null) {
                fail(ErrorKind.BT_REGISTER)
                return
            }
            try {
                val sdp = BluetoothHidDeviceAppSdpSettings(
                    APP_NAME, APP_DESCRIPTION, APP_PROVIDER,
                    (BluetoothHidDevice.SUBCLASS1_NONE.toInt() or BluetoothHidDevice.SUBCLASS2_GAMEPAD.toInt()).toByte(),
                    HidReport.DESCRIPTOR,
                )
                if (!h.registerApp(sdp, null, null, exec, callbacks)) {
                    fail(ErrorKind.BT_REGISTER)
                } else {
                    registered = true
                    _status.value = TransportStatus.Connecting() // ready; pick a host
                    pendingHost?.let { doConnect(h, it) }
                }
            } catch (_: SecurityException) {
                fail(ErrorKind.BT_PERMISSION)
            }
        }

        override fun onServiceDisconnected(profile: Int) {
            if (profile == BluetoothProfile.HID_DEVICE) {
                hid = null
                if (running) fail(ErrorKind.BT_CONNECT)
            }
        }
    }

    private val callbacks = object : BluetoothHidDevice.Callback() {
        override fun onConnectionStateChanged(device: BluetoothDevice, state: Int) {
            when (state) {
                BluetoothProfile.STATE_CONNECTED -> {
                    host = device
                    pendingHost = null
                    connected = true
                    startSender()
                    _status.value = TransportStatus.Connected(MAX_RATE_HZ, safeName(device))
                }
                BluetoothProfile.STATE_DISCONNECTED -> {
                    connected = false
                    stopSender()
                    if (running) _status.value = TransportStatus.Connecting() // waiting for a host
                }
            }
        }

        override fun onGetReport(device: BluetoothDevice, type: Byte, id: Byte, bufferSize: Int) {
            if (id.toInt() != HidReport.REPORT_ID) return
            try {
                hid?.replyReport(device, type, id, report.copyOf())
            } catch (_: SecurityException) {
            } catch (_: Exception) {
            }
        }

        override fun onSetReport(device: BluetoothDevice, type: Byte, id: Byte, data: ByteArray) {}

        override fun onInterruptData(device: BluetoothDevice, reportId: Byte, data: ByteArray) {}
    }

    private fun startSender() {
        if (sender?.isAlive == true) return
        lastSent.clear()
        sender = Thread(::senderLoop, "pp-bt-send").apply {
            isDaemon = true
            start()
        }
    }

    private fun stopSender() {
        senderLock.withLock { senderWake.signalAll() }
        sender?.join(500)
        sender = null
    }

    private fun senderLoop() {
        val schedule = SendSchedule(MAX_RATE_HZ, heartbeatMs = 0, snapshotMs = RESEND_MS)
        val state = ControllerState()
        while (running && connected) {
            val now = nowUs()
            input.snapshot(state)
            val changed = !state.sameOnWire(lastSent)
            val held = !state.isNeutral
            val action = schedule.poll(now, changed, held)
            if (action and SendSchedule.STATE != 0) {
                HidReport.encode(state, report)
                if (trySend(report)) {
                    lastSent.copyFrom(state)
                    schedule.sentState(now)
                }
            }
            val pending = changed && (action and SendSchedule.STATE == 0)
            senderLock.withLock { senderWake.await(schedule.nextDueUs(nowUs(), pending, held), TimeUnit.MICROSECONDS) }
        }
    }

    private fun trySend(data: ByteArray): Boolean {
        val h = hid ?: return false
        val d = host ?: return false
        return try {
            h.sendReport(d, HidReport.REPORT_ID, data)
        } catch (_: SecurityException) {
            false
        } catch (_: Exception) {
            false
        }
    }

    private fun safeName(device: BluetoothDevice): String = try {
        device.name ?: device.address
    } catch (_: SecurityException) {
        device.address
    }

    private fun nowUs(): Long = SystemClock.elapsedRealtimeNanos() / 1000

    companion object {
        private const val APP_NAME = "PocketPad"
        private const val APP_DESCRIPTION = "PocketPad gamepad"
        private const val APP_PROVIDER = "PocketPad"
        private const val MAX_RATE_HZ = 125
        private const val RESEND_MS = 100
    }
}
