package io.pocketpad.app.core

import android.content.Context
import android.os.Build
import android.os.VibrationEffect
import android.os.Vibrator
import android.os.VibratorManager

/**
 * Controller feedback. Uses the platform's predefined haptic effects (crisp
 * "click" for buttons, light "tick" for d-pad turns and trigger thresholds),
 * which are tuned per device and respect the system haptics intensity, so
 * they feel accurate rather than like a generic keyboard tap.
 */
class Haptics(context: Context, private val enabled: () -> Boolean) {

    private val vibrator: Vibrator = if (Build.VERSION.SDK_INT >= 31) {
        (context.getSystemService(Context.VIBRATOR_MANAGER_SERVICE) as VibratorManager).defaultVibrator
    } else {
        @Suppress("DEPRECATION")
        context.getSystemService(Context.VIBRATOR_SERVICE) as Vibrator
    }

    /** Button down: a crisp click. */
    fun click() = play(predefined = VibrationEffect.EFFECT_CLICK, fallbackMs = 14, fallbackAmp = 200)

    /** D-pad direction change or trigger threshold: a light tick. */
    fun tick() = play(predefined = VibrationEffect.EFFECT_TICK, fallbackMs = 8, fallbackAmp = 120)

    private fun play(predefined: Int, fallbackMs: Long, fallbackAmp: Int) {
        if (!enabled()) return
        try {
            val effect = if (Build.VERSION.SDK_INT >= 29) {
                VibrationEffect.createPredefined(predefined)
            } else {
                VibrationEffect.createOneShot(fallbackMs, fallbackAmp)
            }
            vibrator.vibrate(effect)
        } catch (_: SecurityException) {
            // No VIBRATE permission on a broken device: input still works.
        }
    }
}
