package io.pocketpad.app.core

import android.content.Context
import android.os.Build
import android.os.SystemClock
import android.os.VibrationEffect
import android.os.Vibrator
import android.os.VibratorManager
import io.pocketpad.protocol.Wire
import kotlin.math.max

sealed interface RumbleDecision {
    data object Ignore : RumbleDecision
    data object Stop : RumbleDecision
    data class Vibrate(val strength: Float, val durationMs: Int) : RumbleDecision
}

/** The pure rumble decision rule; see docs/HANDOFF.md §4.2 ("Rumbler"). */
object RumblePolicy {
    const val MIN_STRENGTH = 0.02f
    const val NO_AMPLITUDE_THRESHOLD = 0.3f
    const val MIN_GAP_MS = 15L

    fun decide(
        nowMs: Long,
        left: Int,
        right: Int,
        durationMs: Int,
        lastMs: Long,
        amplitudeControl: Boolean,
        enabled: Boolean,
    ): RumbleDecision {
        if (!enabled) return RumbleDecision.Stop
        val strength = max(left, right).coerceIn(0, 0xFFFF) / 65535f
        if (strength < MIN_STRENGTH) return RumbleDecision.Stop
        if (nowMs - lastMs < MIN_GAP_MS) return RumbleDecision.Ignore
        if (!amplitudeControl && strength <= NO_AMPLITUDE_THRESHOLD) return RumbleDecision.Ignore
        return RumbleDecision.Vibrate(strength, durationMs.coerceIn(1, Wire.MAX_RUMBLE_MS))
    }
}

/** Plays receiver rumble on the phone's vibrator. */
class Rumbler(private val context: Context, private val enabled: () -> Boolean) {

    private val vibrator: Vibrator = if (Build.VERSION.SDK_INT >= 31) {
        (context.getSystemService(Context.VIBRATOR_MANAGER_SERVICE) as VibratorManager).defaultVibrator
    } else {
        @Suppress("DEPRECATION")
        context.getSystemService(Context.VIBRATOR_SERVICE) as Vibrator
    }

    private var lastMs = Long.MIN_VALUE / 4

    fun onRumble(left: Int, right: Int, durationMs: Int) {
        val amplitude = vibrator.hasAmplitudeControl()
        when (val d = RumblePolicy.decide(
            SystemClock.elapsedRealtime(), left, right, durationMs, lastMs, amplitude, enabled(),
        )) {
            RumbleDecision.Ignore -> Unit
            RumbleDecision.Stop -> stop()
            is RumbleDecision.Vibrate -> {
                lastMs = SystemClock.elapsedRealtime()
                val effect = if (amplitude) {
                    VibrationEffect.createOneShot(d.durationMs.toLong(), (d.strength * 255f).toInt().coerceIn(1, 255))
                } else {
                    VibrationEffect.createOneShot(d.durationMs.toLong(), VibrationEffect.DEFAULT_AMPLITUDE)
                }
                try {
                    vibrator.vibrate(effect)
                } catch (_: SecurityException) {
                    // VIBRATE permission missing on a broken device: input still works.
                }
            }
        }
    }

    fun stop() {
        vibrator.cancel()
        lastMs = Long.MIN_VALUE / 4
    }
}
