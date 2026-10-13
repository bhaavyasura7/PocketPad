package io.pocketpad.app.core

import kotlinx.coroutines.flow.StateFlow

/** Why a connection attempt failed; the UI maps these to strings. */
enum class ErrorKind {
    NO_ANSWER,
    REJECT,
    SOCKET,
    DISCONNECTED,
    BT_UNAVAILABLE,
    BT_OFF,
    BT_PERMISSION,
    BT_REGISTER,
    BT_CONNECT,
}

sealed interface TransportStatus {
    data object Idle : TransportStatus
    data class Connecting(val attempt: Int = 0, val attempts: Int = 4) : TransportStatus
    data class Connected(val rateHz: Int, val receiver: String?) : TransportStatus
    data class Reconnecting(val inMs: Long = 0) : TransportStatus
    data class Error(val kind: ErrorKind, val detail: String? = null, val reason: Int = -1) : TransportStatus
}

/** Live numbers for the controller overlay. */
data class Stats(val rttUs: Int = 0, val rateHz: Int = 0)

/**
 * A connection that carries controller input to a computer, over Wi-Fi
 * ([WifiTransport]) or Bluetooth HID ([BluetoothHidTransport]).
 */
interface Transport {
    val status: StateFlow<TransportStatus>
    val stats: StateFlow<Stats>

    /** Starts connecting (or waiting for a Bluetooth host). Idempotent. */
    fun start()

    /** Releases everything and stops. Sends neutral + DISCONNECT when possible. */
    fun stop()
}
