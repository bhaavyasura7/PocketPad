package io.pocketpad.app.core

import io.pocketpad.protocol.DPad
import kotlin.math.atan2
import kotlin.math.floor
import kotlin.math.hypot

/**
 * Pure input mappings shared by the touch view and the unit tests.
 * Coordinate conventions: screen coordinates (x right, y down). Protocol
 * coordinates: +X right, +Y up.
 */
object Mapping {

    /**
     * Radial dead zone with rescaling, `m' = (m - dz) / (1 - dz)`.
     * Returns the adjusted (x, y) or null inside the dead zone. The result is
     * clamped to the unit circle.
     */
    fun deadZone(x: Float, y: Float, dz: Float): FloatArray? {
        val m = hypot(x, y)
        if (m <= dz || m == 0f) return null
        val out = FloatArray(2)
        if (m <= 1f) {
            val scale = (m - dz) / ((1f - dz) * m)
            out[0] = x * scale
            out[1] = y * scale
        } else {
            out[0] = x / m
            out[1] = y / m
        }
        return out
    }

    /**
     * Trigger value from the vertical position [p] (0 at the top of the
     * trigger rectangle, 1 at its bottom): the top 10% gives about 0.15 and
     * the bottom 40% (p >= 0.6) gives 1.0. With [analog] off, a touch is a
     * full press.
     */
    fun triggerValue(p: Float, analog: Boolean): Float {
        if (!analog) return 1f
        return 0.15f + 0.85f * ((p - 0.1f) / 0.5f).coerceIn(0f, 1f)
    }

    /**
     * D-pad sector from an offset in screen coordinates (y grows downwards),
     * 45-degree sectors, neutral inside [deadFrac] of the zone radius.
     * Returns a protocol D-pad value: 0 neutral, 1 = up, clockwise to 8.
     */
    fun dpadSector(dx: Float, dy: Float, radius: Float, deadFrac: Float = 0.25f): Int {
        if (hypot(dx, dy) < radius * deadFrac) return DPad.NEUTRAL
        val deg = Math.toDegrees(atan2((-dy).toDouble(), dx.toDouble()))
        val sector = ((floor((deg + 22.5) / 45.0).toInt() % 8) + 8) % 8
        return SECTORS[sector]
    }

    // Sector index 0 = east, increasing counter-clockwise in math coordinates.
    private val SECTORS = intArrayOf(
        DPad.RIGHT, DPad.UP_RIGHT, DPad.UP, DPad.UP_LEFT,
        DPad.LEFT, DPad.DOWN_LEFT, DPad.DOWN, DPad.DOWN_RIGHT,
    )
}
