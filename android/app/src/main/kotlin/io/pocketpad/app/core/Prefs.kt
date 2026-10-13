package io.pocketpad.app.core

import android.content.Context
import java.security.SecureRandom

/**
 * App-private storage for the device ID, saved receivers (with their pairing
 * secrets) and user settings. Secrets are never logged.
 */
class Prefs(context: Context) {

    private val sp = context.getSharedPreferences("pocketpad", Context.MODE_PRIVATE)

    // --- identity ---

    /** Random 64-bit device ID, generated once. */
    val deviceId: Long
        get() {
            val existing = sp.getLong(KEY_DEVICE_ID, 0L)
            if (existing != 0L) return existing
            var v: Long
            do {
                v = SecureRandom().nextLong()
            } while (v == 0L)
            sp.edit().putLong(KEY_DEVICE_ID, v).apply()
            return v
        }

    // --- receivers ---

    data class Receiver(val key: String, val name: String, val secret: ByteArray)

    fun receivers(): List<Receiver> =
        sp.all.keys.filter { it.startsWith(PREFIX_RECEIVER) }.mapNotNull { k ->
            val v = sp.getString(k, null) ?: return@mapNotNull null
            val parts = v.split('|')
            if (parts.size != 2) return@mapNotNull null
            val secret = hexToBytes(parts[1]) ?: return@mapNotNull null
            Receiver(k.removePrefix(PREFIX_RECEIVER), parts[0], secret)
        }.sortedBy { it.key }

    fun saveReceiver(key: String, name: String, secret: ByteArray) {
        val safe = name.replace('|', ' ').take(32)
        sp.edit().putString(PREFIX_RECEIVER + key, "$safe|${bytesToHex(secret)}").apply()
    }

    fun forgetReceiver(key: String) {
        sp.edit().remove(PREFIX_RECEIVER + key).apply()
    }

    fun secretFor(key: String): ByteArray? {
        val v = sp.getString(PREFIX_RECEIVER + key, null) ?: return null
        return hexToBytes(v.split('|').getOrNull(1) ?: return null)
    }

    var lastWifi: String?
        get() = sp.getString(KEY_LAST_WIFI, null)
        set(v) { sp.edit().putString(KEY_LAST_WIFI, v).apply() }

    var lastBtHost: String?
        get() = sp.getString(KEY_LAST_BT, null)
        set(v) { sp.edit().putString(KEY_LAST_BT, v).apply() }

    // --- settings ---

    /** Dead zone as a percent, 5..30. */
    var deadZonePercent: Int
        get() = sp.getInt(KEY_DEAD_ZONE, 10).coerceIn(5, 30)
        set(v) { sp.edit().putInt(KEY_DEAD_ZONE, v.coerceIn(5, 30)).apply() }

    var triggerAnalog: Boolean
        get() = sp.getBoolean(KEY_TRIGGER_ANALOG, true)
        set(v) { sp.edit().putBoolean(KEY_TRIGGER_ANALOG, v).apply() }

    /** Requested update rate; the receiver's WELCOME caps it. */
    var rateHz: Int
        get() = sp.getInt(KEY_RATE, 250).let { if (it in RATES) it else 250 }
        set(v) { sp.edit().putInt(KEY_RATE, if (v in RATES) v else 250).apply() }

    var haptic: Boolean
        get() = sp.getBoolean(KEY_HAPTIC, true)
        set(v) { sp.edit().putBoolean(KEY_HAPTIC, v).apply() }

    var vibrateOnRumble: Boolean
        get() = sp.getBoolean(KEY_VIBRATE, true)
        set(v) { sp.edit().putBoolean(KEY_VIBRATE, v).apply() }

    /** Layout scale as a percent, 80..120. */
    var layoutScalePercent: Int
        get() = sp.getInt(KEY_SCALE, 100).coerceIn(80, 120)
        set(v) { sp.edit().putInt(KEY_SCALE, v.coerceIn(80, 120)).apply() }

    var leftHanded: Boolean
        get() = sp.getBoolean(KEY_LEFT_HANDED, false)
        set(v) { sp.edit().putBoolean(KEY_LEFT_HANDED, v).apply() }

    var stickDoubleTap: Boolean
        get() = sp.getBoolean(KEY_STICK_DOUBLE_TAP, false)
        set(v) { sp.edit().putBoolean(KEY_STICK_DOUBLE_TAP, v).apply() }

    fun uiSettings() = UiSettings(
        deadZone = deadZonePercent / 100f,
        triggerAnalog = triggerAnalog,
        haptic = haptic,
        leftHanded = leftHanded,
        layoutScale = layoutScalePercent / 100f,
        stickDoubleTap = stickDoubleTap,
    )

    fun resetSettings() {
        sp.edit()
            .remove(KEY_DEAD_ZONE).remove(KEY_TRIGGER_ANALOG).remove(KEY_RATE)
            .remove(KEY_HAPTIC).remove(KEY_VIBRATE).remove(KEY_SCALE)
            .remove(KEY_LEFT_HANDED).remove(KEY_STICK_DOUBLE_TAP)
            .apply()
    }

    companion object {
        val RATES = intArrayOf(120, 166, 250)

        private const val KEY_DEVICE_ID = "deviceId"
        private const val KEY_LAST_WIFI = "lastWifi"
        private const val KEY_LAST_BT = "lastBtHost"
        private const val KEY_DEAD_ZONE = "deadZone"
        private const val KEY_TRIGGER_ANALOG = "triggerAnalog"
        private const val KEY_RATE = "rate"
        private const val KEY_HAPTIC = "haptic"
        private const val KEY_VIBRATE = "vibrate"
        private const val KEY_SCALE = "scale"
        private const val KEY_LEFT_HANDED = "leftHanded"
        private const val KEY_STICK_DOUBLE_TAP = "stickDoubleTap"
        private const val PREFIX_RECEIVER = "recv:"

        fun bytesToHex(b: ByteArray): String {
            val sb = StringBuilder(b.size * 2)
            for (x in b) sb.append("%02x".format(x))
            return sb.toString()
        }

        fun hexToBytes(s: String): ByteArray? {
            if (s.length != 30 || s.length % 2 != 0) return null // 15-byte secrets
            val out = ByteArray(s.length / 2)
            for (i in out.indices) {
                val v = s.substring(i * 2, i * 2 + 2).toIntOrNull(16) ?: return null
                out[i] = v.toByte()
            }
            return out
        }
    }
}
