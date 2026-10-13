package io.pocketpad.app.core

import kotlin.math.hypot

/** A rectangle in view pixels. */
class Bounds {
    @JvmField var left = 0f
    @JvmField var top = 0f
    @JvmField var right = 0f
    @JvmField var bottom = 0f

    val width get() = right - left
    val height get() = bottom - top

    fun set(l: Float, t: Float, r: Float, b: Float) {
        left = l; top = t; right = r; bottom = b
    }
}

/** A circle in view pixels. */
class Circle {
    @JvmField var cx = 0f
    @JvmField var cy = 0f
    @JvmField var r = 0f

    fun set(cx: Float, cy: Float, r: Float) {
        this.cx = cx; this.cy = cy; this.r = r
    }

    fun hit(x: Float, y: Float, grow: Float = 0f): Boolean =
        hypot(x - cx, y - cy) <= r * (1f + grow)
}

/**
 * The on-screen control rectangles and circles for one view size. Coordinates
 * are computed in pixels from height-derived fractions, so circles stay round
 * on any display. [leftHanded] mirrors every control horizontally (labels
 * keep their meaning: the left trigger stays the left trigger).
 *
 * Pure Kotlin (no android.graphics) so it is unit tested on the JVM.
 */
class Layout {
    @JvmField val lt = Bounds()
    @JvmField val rt = Bounds()
    @JvmField val lb = Bounds()
    @JvmField val rb = Bounds()
    @JvmField val leftStick = Circle()
    @JvmField val rightStick = Circle()
    @JvmField val dpad = Circle()
    @JvmField val a = Circle()
    @JvmField val b = Circle()
    @JvmField val x = Circle()
    @JvmField val y = Circle()
    @JvmField val back = Circle()
    @JvmField val start = Circle()
    @JvmField val guide = Circle()
    @JvmField val lsClick = Circle()
    @JvmField val rsClick = Circle()

    fun compute(w: Float, h: Float, scale: Float, leftHanded: Boolean) {
        if (w <= 0f || h <= 0f) return
        val s = scale.coerceIn(0.5f, 1.5f)

        // Kept away from the display edges: rounded corners, curved glass and
        // the gesture areas cannot be touched reliably on edge-to-edge
        // phones. The top strip y < 0.075 is reserved for the status overlay
        // chips (centre-left and centre-right), clear of the bumper columns.
        val ltX0 = 0.05f * w
        val ltX1 = 0.16f * w
        val ltY0 = 0.075f * h
        val lbY1 = 0.145f * h
        val ltY1 = 0.32f * h
        rect(lb, ltX0, ltY0, ltX1, lbY1, w, h, s, leftHanded)
        rect(rb, w - ltX1, ltY0, w - ltX0, lbY1, w, h, s, leftHanded)
        rect(lt, ltX0, 0.155f * h, ltX1, ltY1, w, h, s, leftHanded)
        rect(rt, w - ltX1, 0.155f * h, w - ltX0, ltY1, w, h, s, leftHanded)

        circle(back, cx = 0.40f * w, cy = 0.145f * h, r = 0.045f * h, w, h, s, leftHanded)
        circle(guide, cx = 0.50f * w, cy = 0.125f * h, r = 0.052f * h, w, h, s, leftHanded)
        circle(start, cx = 0.60f * w, cy = 0.145f * h, r = 0.045f * h, w, h, s, leftHanded)

        // The d-pad stops above the left stick; the ABXY diamond stops above
        // the right trigger rectangle. Both gaps are asserted in LayoutTest.
        circle(dpad, cx = 0.155f * w, cy = 0.495f * h, r = 0.12f * h, w, h, s, leftHanded)
        circle(leftStick, cx = 0.155f * w, cy = 0.77f * h, r = 0.145f * h, w, h, s, leftHanded)
        circle(rightStick, cx = 0.60f * w, cy = 0.77f * h, r = 0.145f * h, w, h, s, leftHanded)
        circle(lsClick, cx = 0.30f * w, cy = 0.87f * h, r = 0.04f * h, w, h, s, leftHanded)
        circle(rsClick, cx = 0.44f * w, cy = 0.87f * h, r = 0.04f * h, w, h, s, leftHanded)

        val acx = 0.845f * w
        val acy = 0.52f * h
        val ar = 0.06f * h
        val off = 0.11f * h
        circle(a, cx = acx, cy = acy + off, r = ar, w, h, s, leftHanded)
        circle(b, cx = acx + off, cy = acy, r = ar, w, h, s, leftHanded)
        circle(x, cx = acx - off, cy = acy, r = ar, w, h, s, leftHanded)
        circle(y, cx = acx, cy = acy - off, r = ar, w, h, s, leftHanded)
    }

    private fun rect(r: Bounds, l: Float, t: Float, rr: Float, bb: Float, w: Float, h: Float, s: Float, mirror: Boolean) {
        var l2 = l; var r2 = rr
        if (mirror) { val nl = w - r2; r2 = w - l2; l2 = nl }
        r.set(
            sx(l2, w, s), sy(t, h, s), sx(r2, w, s), sy(bb, h, s),
        )
    }

    private fun circle(c: Circle, cx: Float, cy: Float, r: Float, w: Float, h: Float, s: Float, mirror: Boolean) {
        val x2 = if (mirror) w - cx else cx
        c.set(sx(x2, w, s), sy(cy, h, s), r * s)
    }

    private fun sx(x: Float, w: Float, s: Float) = w / 2f + (x - w / 2f) * s
    private fun sy(y: Float, h: Float, s: Float) = h / 2f + (y - h / 2f) * s
}
