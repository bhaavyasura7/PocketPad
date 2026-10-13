package io.pocketpad.protocol

import kotlin.math.floor

/** PocketPad protocol v1 constants. See docs/protocol.md. All multibyte fields are little-endian. */
object Wire {
    const val MAGIC0: Byte = 'P'.code.toByte()
    const val MAGIC1: Byte = 'P'.code.toByte()
    const val VERSION = 1
    const val MIN_VERSION = 1
    const val MAX_VERSION = 1
    const val DEFAULT_PORT = 47770

    const val HEADER_SIZE = 6
    const val TAG_SIZE = 8
    const val HANDSHAKE_TAG_SIZE = 16
    const val NONCE_SIZE = 16

    const val STATE_SIZE = 20
    const val HEARTBEAT_SIZE = 20
    const val DISCONNECT_SIZE = 8
    const val RUMBLE_SIZE = 12
    const val DISCOVER_SIZE = 6
    const val DISCOVER_REPLY_MIN = 13
    const val HELLO_SIZE = 58
    const val WELCOME_SIZE = 36
    const val REJECT_SIZE = 10
    const val DEVICE_NAME_SIZE = 24
    const val MAX_SERVER_NAME_SIZE = 32
    const val MAX_DATAGRAM = 128

    const val TYPE_STATE = 0x01
    const val TYPE_HEARTBEAT = 0x02
    const val TYPE_DISCONNECT = 0x03
    const val TYPE_RUMBLE = 0x04
    const val TYPE_DISCOVER = 0x10
    const val TYPE_DISCOVER_REPLY = 0x11
    const val TYPE_HELLO = 0x12
    const val TYPE_WELCOME = 0x13
    const val TYPE_REJECT = 0x14

    const val CAP_RUMBLE = 1
    const val CAP_BATTERY = 2
    const val CAP_GUIDE = 4
    const val CAP_LATENCY_ECHO = 8

    const val FLAG_SNAPSHOT = 1

    const val REASON_USER = 0
    const val REASON_SHUTDOWN = 1
    const val REASON_TIMEOUT = 2
    const val REASON_REPLACED = 3

    const val REJECT_VERSION = 1
    const val REJECT_BUSY = 2
    const val REJECT_RATE_LIMITED = 3

    const val BATTERY_UNKNOWN = 0xFF
    const val HEARTBEAT_CHARGING = 1
    const val MAX_RUMBLE_MS = 2000

    fun isSessionType(t: Int) = t in TYPE_STATE..TYPE_RUMBLE

    /** Full datagram size for a fixed-size type including its tag, or 0. */
    fun wireSize(t: Int): Int = when (t) {
        TYPE_STATE -> STATE_SIZE + TAG_SIZE
        TYPE_HEARTBEAT -> HEARTBEAT_SIZE + TAG_SIZE
        TYPE_DISCONNECT -> DISCONNECT_SIZE + TAG_SIZE
        TYPE_RUMBLE -> RUMBLE_SIZE + TAG_SIZE
        TYPE_DISCOVER -> DISCOVER_SIZE
        TYPE_HELLO -> HELLO_SIZE + HANDSHAKE_TAG_SIZE
        TYPE_WELCOME -> WELCOME_SIZE + HANDSHAKE_TAG_SIZE
        TYPE_REJECT -> REJECT_SIZE
        else -> 0
    }
}

/** Button mask bits. */
object Buttons {
    const val A = 1 shl 0
    const val B = 1 shl 1
    const val X = 1 shl 2
    const val Y = 1 shl 3
    const val LB = 1 shl 4
    const val RB = 1 shl 5
    const val BACK = 1 shl 6
    const val START = 1 shl 7
    const val LS = 1 shl 8
    const val RS = 1 shl 9
    const val GUIDE = 1 shl 10
    const val DEFINED = (1 shl 11) - 1
}

/** D-pad values: 0 neutral, 1..8 clockwise from up. */
object DPad {
    const val NEUTRAL = 0
    const val UP = 1
    const val UP_RIGHT = 2
    const val RIGHT = 3
    const val DOWN_RIGHT = 4
    const val DOWN = 5
    const val DOWN_LEFT = 6
    const val LEFT = 7
    const val UP_LEFT = 8

    fun from(up0: Boolean, down0: Boolean, left0: Boolean, right0: Boolean): Int {
        var up = up0; var down = down0; var left = left0; var right = right0
        if (up && down) { up = false; down = false }
        if (left && right) { left = false; right = false }
        return when {
            up && right -> UP_RIGHT
            up && left -> UP_LEFT
            down && right -> DOWN_RIGHT
            down && left -> DOWN_LEFT
            up -> UP
            down -> DOWN
            left -> LEFT
            right -> RIGHT
            else -> NEUTRAL
        }
    }

    fun up(d: Int) = d == UP || d == UP_RIGHT || d == UP_LEFT
    fun down(d: Int) = d == DOWN || d == DOWN_RIGHT || d == DOWN_LEFT
    fun left(d: Int) = d == LEFT || d == UP_LEFT || d == DOWN_LEFT
    fun right(d: Int) = d == RIGHT || d == UP_RIGHT || d == DOWN_RIGHT
}

/**
 * Canonical normalized controller state. Axes in [-1, 1] with +Y up; triggers in [0, 1].
 * Mutable so the input path can reuse one instance instead of allocating per update.
 */
class ControllerState {
    @JvmField var lx = 0f
    @JvmField var ly = 0f
    @JvmField var rx = 0f
    @JvmField var ry = 0f
    @JvmField var lt = 0f
    @JvmField var rt = 0f
    @JvmField var buttons = 0
    @JvmField var dpad = DPad.NEUTRAL

    fun clear() {
        lx = 0f; ly = 0f; rx = 0f; ry = 0f; lt = 0f; rt = 0f; buttons = 0; dpad = DPad.NEUTRAL
    }

    fun copyFrom(o: ControllerState) {
        lx = o.lx; ly = o.ly; rx = o.rx; ry = o.ry; lt = o.lt; rt = o.rt; buttons = o.buttons; dpad = o.dpad
    }

    val isNeutral: Boolean
        get() = lx == 0f && ly == 0f && rx == 0f && ry == 0f && lt == 0f && rt == 0f && buttons == 0 && dpad == DPad.NEUTRAL

    /** Equality at wire precision, which is what decides whether a packet is needed. */
    fun sameOnWire(o: ControllerState): Boolean =
        buttons == o.buttons && dpad == o.dpad &&
            Codec.encodeAxis(lx) == Codec.encodeAxis(o.lx) && Codec.encodeAxis(ly) == Codec.encodeAxis(o.ly) &&
            Codec.encodeAxis(rx) == Codec.encodeAxis(o.rx) && Codec.encodeAxis(ry) == Codec.encodeAxis(o.ry) &&
            Codec.encodeTrigger(lt) == Codec.encodeTrigger(o.lt) && Codec.encodeTrigger(rt) == Codec.encodeTrigger(o.rt)

    override fun equals(other: Any?) = other is ControllerState &&
        lx == other.lx && ly == other.ly && rx == other.rx && ry == other.ry &&
        lt == other.lt && rt == other.rt && buttons == other.buttons && dpad == other.dpad

    override fun hashCode() = buttons * 31 + dpad

    override fun toString() = "State(L=$lx,$ly R=$rx,$ry LT=$lt RT=$rt buttons=0x${buttons.toString(16)} dpad=$dpad)"
}

/** Validation failure. [kind] matches the names used in the shared test vectors. */
class ProtocolException(val kind: String) : Exception(kind)

/** Sequence-number serial arithmetic (RFC 1982) over 16 bits. */
fun seqNewer(a: Int, b: Int): Boolean = ((a - b) and 0xFFFF).toShort() > 0

internal fun roundHalfAway(x: Double): Double = if (x < 0) -floor(-x + 0.5) else floor(x + 0.5)
