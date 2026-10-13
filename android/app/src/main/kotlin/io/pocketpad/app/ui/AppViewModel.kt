package io.pocketpad.app.ui

import android.app.Application
import android.bluetooth.BluetoothDevice
import android.os.Build
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.AndroidViewModel
import io.pocketpad.app.core.BatteryReader
import io.pocketpad.app.core.BluetoothHidTransport
import io.pocketpad.app.core.InputState
import io.pocketpad.app.core.Prefs
import io.pocketpad.app.core.Rumbler
import io.pocketpad.app.core.Transport
import io.pocketpad.app.core.UiSettings
import io.pocketpad.app.core.WifiTransport
import io.pocketpad.protocol.Pairing

enum class Screen { Connect, Controller, Settings, Controls }

class AppViewModel(app: Application) : AndroidViewModel(app) {

    val prefs = Prefs(app)
    val input = InputState()
    val battery = BatteryReader(app)
    val rumbler = Rumbler(app) { prefs.vibrateOnRumble }

    var screen by mutableStateOf(Screen.Connect)
        private set

    var transport by mutableStateOf<Transport?>(null)
        private set

    var bt by mutableStateOf<BluetoothHidTransport?>(null)
        private set

    /** Bumped when settings change so composables re-read [uiSettings]. */
    private var settingsVersion by mutableIntStateOf(0)

    fun uiSettings(): UiSettings {
        settingsVersion // subscribe
        return prefs.uiSettings()
    }

    fun settingsChanged() {
        settingsVersion++
    }

    fun connectWifi(host: String, port: Int, secret: ByteArray, name: String) {
        val key = "$host:$port"
        prefs.saveReceiver(key, name, secret)
        prefs.lastWifi = key
        val t = WifiTransport(
            host = host,
            port = port,
            pairKey = Pairing.pairKey(secret),
            deviceId = prefs.deviceId,
            deviceName = Build.MODEL ?: "Android",
            requestedRateHz = prefs.rateHz,
            input = input,
            rumbler = rumbler,
            battery = battery::read,
        )
        attach(t)
    }

    fun bluetooth(): BluetoothHidTransport {
        bt?.let { return it }
        return BluetoothHidTransport(getApplication(), input).also { bt = it }
    }

    fun connectBluetooth(device: BluetoothDevice) {
        val b = bluetooth()
        prefs.lastBtHost = device.address
        b.start()
        b.connect(device)
        attach(b)
    }

    private fun attach(t: Transport) {
        if (transport !== t) {
            transport?.stop()
        }
        transport = t
        t.start()
        screen = Screen.Controller
    }

    fun exitController() {
        transport?.stop()
        transport = null
        input.clear()
        screen = Screen.Connect
    }

    fun openSettings() {
        screen = Screen.Settings
    }

    fun closeSettings() {
        screen = if (transport != null) Screen.Controller else Screen.Connect
    }

    fun openControls() {
        screen = Screen.Controls
    }

    fun closeControls() {
        screen = if (transport != null) Screen.Controller else Screen.Connect
    }

    override fun onCleared() {
        transport?.stop()
        bt?.stop()
    }
}
