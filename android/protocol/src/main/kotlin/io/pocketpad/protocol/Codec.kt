package io.pocketpad.protocol

/** Little-endian packet encoding and decoding into caller-owned buffers. Nothing here allocates on success. */
object Codec {
    fun encodeAxis(v: Float): Int {
        if (v.isNaN()) return 0
        if (v >= 1f) return 32767
        if (v <= -1f) return -32767
        return roundHalfAway(v.toDouble() * 32767).toInt()
    }

    fun decodeAxis(v: Int): Float = if (v <= -32767) -1f else v / 32767f

    fun encodeTrigger(v: Float): Int {
        if (v.isNaN() || v <= 0f) return 0
        if (v >= 1f) return 255
        return roundHalfAway(v.toDouble() * 255).toInt()
    }

    fun decodeTrigger(v: Int): Float = v / 255f

    // --- little-endian helpers ---

    fun put16(b: ByteArray, off: Int, v: Int) {
        b[off] = v.toByte(); b[off + 1] = (v shr 8).toByte()
    }

    fun put32(b: ByteArray, off: Int, v: Int) {
        b[off] = v.toByte(); b[off + 1] = (v shr 8).toByte(); b[off + 2] = (v shr 16).toByte(); b[off + 3] = (v shr 24).toByte()
    }

    fun put64(b: ByteArray, off: Int, v: Long) {
        put32(b, off, v.toInt()); put32(b, off + 4, (v ushr 32).toInt())
    }

    fun u8(b: ByteArray, off: Int) = b[off].toInt() and 0xFF
    fun u16(b: ByteArray, off: Int) = u8(b, off) or (u8(b, off + 1) shl 8)
    fun s16(b: ByteArray, off: Int) = u16(b, off).toShort().toInt()
    fun u32(b: ByteArray, off: Int): Long = (u16(b, off).toLong()) or (u16(b, off + 2).toLong() shl 16)
    fun s64(b: ByteArray, off: Int): Long = u32(b, off) or (u32(b, off + 4) shl 32)

    // --- header ---

    fun putHeader(b: ByteArray, type: Int, seq: Int) {
        b[0] = Wire.MAGIC0; b[1] = Wire.MAGIC1; b[2] = Wire.VERSION.toByte(); b[3] = type.toByte()
        put16(b, 4, seq)
    }

    fun seqOf(b: ByteArray) = u16(b, 4)
    fun typeOf(b: ByteArray) = u8(b, 3)
    fun versionOf(b: ByteArray) = u8(b, 2)

    /** Checks magic and size bounds only, so foreign versions can be recognised. Returns the type. */
    fun parseHeader(b: ByteArray, len: Int): Int {
        if (len < Wire.HEADER_SIZE) throw ProtocolException("short")
        if (len > Wire.MAX_DATAGRAM) throw ProtocolException("long")
        if (b[0] != Wire.MAGIC0 || b[1] != Wire.MAGIC1) throw ProtocolException("magic")
        return typeOf(b)
    }

    /** Full header validation: magic, version, type, and exact length for the type. Returns the type. */
    fun checkHeader(b: ByteArray, len: Int): Int {
        val t = parseHeader(b, len)
        val v = versionOf(b)
        if (v < Wire.MIN_VERSION || v > Wire.MAX_VERSION) throw ProtocolException("version")
        if (t == Wire.TYPE_DISCOVER_REPLY) {
            if (len < Wire.DISCOVER_REPLY_MIN || len > Wire.DISCOVER_REPLY_MIN + Wire.MAX_SERVER_NAME_SIZE) throw ProtocolException("length")
            return t
        }
        val want = Wire.wireSize(t)
        if (want == 0) throw ProtocolException("type")
        if (len != want) throw ProtocolException("length")
        return t
    }

    // --- STATE ---

    fun putStateRaw(b: ByteArray, seq: Int, buttons: Int, dpad: Int, flags: Int, lx: Int, ly: Int, rx: Int, ry: Int, lt: Int, rt: Int) {
        putHeader(b, Wire.TYPE_STATE, seq)
        put16(b, 6, buttons and Buttons.DEFINED)
        b[8] = dpad.toByte(); b[9] = flags.toByte()
        put16(b, 10, lx); put16(b, 12, ly); put16(b, 14, rx); put16(b, 16, ry)
        b[18] = lt.toByte(); b[19] = rt.toByte()
    }

    fun putState(b: ByteArray, seq: Int, s: ControllerState, flags: Int) = putStateRaw(
        b, seq, s.buttons, s.dpad, flags,
        encodeAxis(s.lx), encodeAxis(s.ly), encodeAxis(s.rx), encodeAxis(s.ry),
        encodeTrigger(s.lt), encodeTrigger(s.rt),
    )

    /** Decodes and validates a STATE body (header type and version included) into [out]. Returns the flags. */
    fun decodeState(b: ByteArray, len: Int, out: ControllerState): Int {
        if (len < Wire.STATE_SIZE) throw ProtocolException("short")
        if (b[0] != Wire.MAGIC0 || b[1] != Wire.MAGIC1) throw ProtocolException("magic")
        if (versionOf(b) != Wire.VERSION) throw ProtocolException("version")
        if (typeOf(b) != Wire.TYPE_STATE) throw ProtocolException("type")
        val buttons = u16(b, 6)
        val dpad = u8(b, 8)
        val flags = u8(b, 9)
        if (buttons and Buttons.DEFINED.inv() != 0 || flags and Wire.FLAG_SNAPSHOT.inv() != 0) throw ProtocolException("reserved")
        if (dpad > DPad.UP_LEFT) throw ProtocolException("range")
        out.buttons = buttons; out.dpad = dpad
        out.lx = decodeAxis(s16(b, 10)); out.ly = decodeAxis(s16(b, 12))
        out.rx = decodeAxis(s16(b, 14)); out.ry = decodeAxis(s16(b, 16))
        out.lt = decodeTrigger(u8(b, 18)); out.rt = decodeTrigger(u8(b, 19))
        return flags
    }

    // --- HEARTBEAT ---

    fun putHeartbeat(b: ByteArray, seq: Int, timeUs: Int, echoUs: Int, rttUs: Int, battery: Int, flags: Int) {
        putHeader(b, Wire.TYPE_HEARTBEAT, seq)
        put32(b, 6, timeUs); put32(b, 10, echoUs); put32(b, 14, rttUs)
        b[18] = battery.toByte(); b[19] = flags.toByte()
    }

    /** Validates a HEARTBEAT body; read the fields with the accessors below. */
    fun checkHeartbeat(b: ByteArray) {
        val bat = u8(b, 18)
        if (bat > 100 && bat != Wire.BATTERY_UNKNOWN) throw ProtocolException("range")
        if (u8(b, 19) and Wire.HEARTBEAT_CHARGING.inv() != 0) throw ProtocolException("reserved")
    }

    fun heartbeatTimeUs(b: ByteArray) = u32(b, 6).toInt()
    fun heartbeatEchoUs(b: ByteArray) = u32(b, 10).toInt()

    // --- DISCONNECT ---

    fun putDisconnect(b: ByteArray, seq: Int, reason: Int) {
        putHeader(b, Wire.TYPE_DISCONNECT, seq)
        b[6] = reason.toByte(); b[7] = 0
    }

    fun decodeDisconnect(b: ByteArray): Int {
        if (b[7].toInt() != 0) throw ProtocolException("reserved")
        val r = u8(b, 6)
        if (r > Wire.REASON_REPLACED) throw ProtocolException("range")
        return r
    }

    // --- RUMBLE ---

    fun putRumble(b: ByteArray, seq: Int, left: Int, right: Int, durationMs: Int) {
        putHeader(b, Wire.TYPE_RUMBLE, seq)
        put16(b, 6, left); put16(b, 8, right); put16(b, 10, durationMs)
    }

    fun rumbleLeft(b: ByteArray) = u16(b, 6)
    fun rumbleRight(b: ByteArray) = u16(b, 8)

    fun rumbleDurationMs(b: ByteArray): Int {
        val d = u16(b, 10)
        if (d > Wire.MAX_RUMBLE_MS) throw ProtocolException("range")
        return d
    }

    // --- DISCOVER / REPLY ---

    fun putDiscover(b: ByteArray) = putHeader(b, Wire.TYPE_DISCOVER, 0)

    class DiscoverReply(val port: Int, val caps: Int, val minVersion: Int, val maxVersion: Int, val name: String)

    fun decodeDiscoverReply(b: ByteArray, len: Int): DiscoverReply {
        if (len < Wire.DISCOVER_REPLY_MIN) throw ProtocolException("short")
        val n = u8(b, 12)
        if (n > Wire.MAX_SERVER_NAME_SIZE || len != Wire.DISCOVER_REPLY_MIN + n) throw ProtocolException("length")
        return DiscoverReply(u16(b, 6), u16(b, 8), u8(b, 10), u8(b, 11), String(b, 13, n, Charsets.UTF_8))
    }

    // --- HELLO / WELCOME / REJECT ---

    /** Encodes a HELLO body (58 bytes). The name is truncated to 24 bytes at a character boundary. */
    fun putHello(b: ByteArray, deviceId: Long, clientNonce: ByteArray, caps: Int, deviceName: String) {
        putHeader(b, Wire.TYPE_HELLO, 0)
        put64(b, 6, deviceId)
        System.arraycopy(clientNonce, 0, b, 14, Wire.NONCE_SIZE)
        b[30] = Wire.MIN_VERSION.toByte(); b[31] = Wire.MAX_VERSION.toByte()
        put16(b, 32, caps)
        val name = truncateUtf8(deviceName, Wire.DEVICE_NAME_SIZE)
        java.util.Arrays.fill(b, 34, 58, 0)
        System.arraycopy(name, 0, b, 34, name.size)
    }

    fun truncateUtf8(s: String, max: Int): ByteArray {
        var out = s.toByteArray(Charsets.UTF_8)
        if (out.size <= max) return out
        var n = max
        // Back off over continuation bytes so a character is never split.
        while (n > 0 && (out[n].toInt() and 0xC0) == 0x80) n--
        out = out.copyOf(n)
        return out
    }

    class Welcome(
        val sessionId: Int, val serverNonce: ByteArray, val version: Int, val caps: Int,
        val heartbeatMs: Int, val timeoutMs: Int, val maxRateHz: Int,
    )

    fun decodeWelcome(b: ByteArray): Welcome {
        if (b[27].toInt() != 0) throw ProtocolException("reserved")
        val w = Welcome(
            sessionId = u32(b, 6).toInt(), serverNonce = b.copyOfRange(10, 26), version = u8(b, 26),
            caps = u16(b, 28), heartbeatMs = u16(b, 30), timeoutMs = u16(b, 32), maxRateHz = u16(b, 34),
        )
        if (w.version < Wire.MIN_VERSION || w.version > Wire.MAX_VERSION || w.heartbeatMs == 0 || w.timeoutMs == 0 || w.maxRateHz == 0) {
            throw ProtocolException("range")
        }
        return w
    }

    class Reject(val reason: Int, val minVersion: Int, val maxVersion: Int)

    fun decodeReject(b: ByteArray, len: Int): Reject {
        if (len < Wire.REJECT_SIZE) throw ProtocolException("short")
        val r = Reject(u8(b, 6), u8(b, 7), u8(b, 8))
        if (r.reason == 0 || r.reason > Wire.REJECT_RATE_LIMITED) throw ProtocolException("range")
        return r
    }

    /** Full pre-authentication validation, mirroring the Go receiver. Used by the vector tests. */
    fun checkDatagram(b: ByteArray, len: Int) {
        when (checkHeader(b, len)) {
            Wire.TYPE_STATE -> decodeState(b, len, ControllerState())
            Wire.TYPE_HEARTBEAT -> checkHeartbeat(b)
            Wire.TYPE_DISCONNECT -> decodeDisconnect(b)
            Wire.TYPE_RUMBLE -> rumbleDurationMs(b)
            Wire.TYPE_WELCOME -> decodeWelcome(b)
            Wire.TYPE_REJECT -> decodeReject(b, len)
            Wire.TYPE_DISCOVER_REPLY -> decodeDiscoverReply(b, len)
        }
    }
}
