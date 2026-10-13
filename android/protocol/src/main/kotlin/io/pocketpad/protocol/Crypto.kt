package io.pocketpad.protocol

import java.security.MessageDigest
import java.security.SecureRandom
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

private fun hmac(key: ByteArray, vararg parts: ByteArray): ByteArray {
    val m = Mac.getInstance("HmacSHA256")
    m.init(SecretKeySpec(key, "HmacSHA256"))
    for (p in parts) m.update(p)
    return m.doFinal()
}

/** Pairing codes: 120-bit secrets written as 24 Crockford base32 characters. */
object Pairing {
    const val SECRET_BYTES = 15
    private const val ALPHABET = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

    fun format(secret: ByteArray): String {
        require(secret.size == SECRET_BYTES)
        val sb = StringBuilder()
        var buf = 0
        var bits = 0
        for (byte in secret) {
            buf = (buf shl 8) or (byte.toInt() and 0xFF)
            bits += 8
            while (bits >= 5) {
                bits -= 5
                sb.append(ALPHABET[(buf shr bits) and 31])
            }
        }
        return sb.chunked(4).joinToString("-")
    }

    /** Parses a code in any case, ignoring separators and reading O as 0 and I/L as 1. Returns null if invalid. */
    fun parse(code: String): ByteArray? {
        val out = ByteArray(SECRET_BYTES)
        var n = 0
        var buf = 0
        var bits = 0
        var chars = 0
        for (c0 in code.uppercase()) {
            val c = when (c0) {
                '-', ' ', '\t', '\n' -> continue
                'O' -> '0'
                'I', 'L' -> '1'
                else -> c0
            }
            val v = ALPHABET.indexOf(c)
            if (v < 0) return null
            chars++
            buf = (buf shl 5) or v
            bits += 5
            if (bits >= 8) {
                bits -= 8
                if (n >= SECRET_BYTES) return null
                out[n++] = (buf shr bits).toByte()
            }
        }
        return if (chars == 24 && n == SECRET_BYTES) out else null
    }

    fun pairKey(secret: ByteArray): ByteArray = hmac("PocketPad v1 pairing key".toByteArray(), secret)
}

object Handshake {
    private val random = SecureRandom()

    fun nonce(): ByteArray = ByteArray(Wire.NONCE_SIZE).also { random.nextBytes(it) }

    fun randomLong(): Long = random.nextLong()

    /** Tag for a HELLO (bound = empty) or WELCOME (bound = client nonce). */
    fun tag(pairKey: ByteArray, body: ByteArray, bodyLen: Int, bound: ByteArray): ByteArray =
        hmac(pairKey, body.copyOf(bodyLen), bound).copyOf(Wire.HANDSHAKE_TAG_SIZE)

    fun verify(pairKey: ByteArray, pkt: ByteArray, len: Int, bound: ByteArray): Boolean {
        val n = len - Wire.HANDSHAKE_TAG_SIZE
        if (n < Wire.HEADER_SIZE) return false
        return MessageDigest.isEqual(tag(pairKey, pkt, n, bound), pkt.copyOfRange(n, len))
    }

    class Keys(val clientToServer: ByteArray, val serverToClient: ByteArray)

    fun deriveSessionKeys(pairKey: ByteArray, deviceId: Long, clientNonce: ByteArray, serverNonce: ByteArray, sessionId: Int): Keys {
        val ids = ByteArray(12)
        Codec.put64(ids, 0, deviceId)
        Codec.put32(ids, 8, sessionId)
        val master = hmac(pairKey, "PocketPad v1 session".toByteArray(), clientNonce, serverNonce, ids)
        return Keys(hmac(master, "c2s".toByteArray()), hmac(master, "s2c".toByteArray()))
    }
}

/** Session-packet tags for one direction. Reuses its buffers; not thread-safe. */
class SessionMac(key: ByteArray) {
    private val mac = Mac.getInstance("HmacSHA256").apply { init(SecretKeySpec(key, "HmacSHA256")) }
    private val roc = ByteArray(4)
    private val sum = ByteArray(32)

    private fun compute(b: ByteArray, bodyLen: Int, ext: Long) {
        Codec.put32(roc, 0, (ext ushr 16).toInt())
        mac.update(roc)
        mac.update(b, 0, bodyLen)
        mac.doFinal(sum, 0)
    }

    fun seal(b: ByteArray, bodyLen: Int, ext: Long) {
        compute(b, bodyLen, ext)
        System.arraycopy(sum, 0, b, bodyLen, Wire.TAG_SIZE)
    }

    fun verify(b: ByteArray, len: Int, ext: Long): Boolean {
        val n = len - Wire.TAG_SIZE
        if (n < Wire.HEADER_SIZE) return false
        compute(b, n, ext)
        var diff = 0
        for (i in 0 until Wire.TAG_SIZE) diff = diff or (sum[i].toInt() xor b[n + i].toInt())
        return diff == 0
    }
}

/** Allocates outgoing extended sequence numbers and tags packets. */
class Sealer(key: ByteArray) {
    private val mac = SessionMac(key)
    private var next = 0L

    /** Returns the next extended sequence number; its low 16 bits go in the header. */
    fun next(): Long = next++

    /** Tags the body and returns the datagram length. */
    fun seal(b: ByteArray, bodyLen: Int, ext: Long): Int {
        mac.seal(b, bodyLen, ext)
        return bodyLen + Wire.TAG_SIZE
    }
}

/** Tracks the highest accepted sequence number; only strictly newer packets are accepted. */
class ReplayWindow {
    private var highest = 0L
    private var has = false

    /** Extended sequence number for [seq], or -1 if it is not newer than everything accepted. */
    fun estimate(seq: Int): Long {
        if (!has) return seq.toLong()
        val delta = ((seq - highest.toInt()) and 0xFFFF).toShort().toInt()
        return if (delta <= 0) -1 else highest + delta
    }

    /** Records [ext] and returns how many sequence numbers were skipped. */
    fun accept(ext: Long): Long {
        val gap = if (has && ext > highest) ext - highest - 1 else 0
        if (!has || ext > highest) { highest = ext; has = true }
        return gap
    }
}

/** Verifies incoming session packets for one direction. */
class Opener(key: ByteArray) {
    private val mac = SessionMac(key)
    private val window = ReplayWindow()

    /** Returns the gap since the last accepted packet; throws "stale" or "auth". Header must be checked. */
    fun open(b: ByteArray, len: Int): Long {
        val ext = window.estimate(Codec.seqOf(b))
        if (ext < 0) throw ProtocolException("stale")
        if (!mac.verify(b, len, ext)) throw ProtocolException("auth")
        return window.accept(ext)
    }
}
