package io.pocketpad.app

import io.pocketpad.app.core.Layout
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class LayoutTest {

    @Test
    fun positionsOnA2000x1000Screen() {
        val l = Layout()
        l.compute(2000f, 1000f, 1f, leftHanded = false)
        assertEquals(310f, l.leftStick.cx, 1f) // 0.155 * w
        assertEquals(770f, l.leftStick.cy, 1f) // 0.77 * h
        assertEquals(145f, l.leftStick.r, 1f) // 0.145 * h
        assertEquals(1690f, l.a.cx, 1f) // 0.845 * w
        assertEquals(495f, l.dpad.cy, 1f) // 0.495 * h
        assertTrue(l.leftStick.hit(310f, 770f))
        assertFalse(l.leftStick.hit(620f, 770f))
        assertTrue(l.dpad.hit(310f, 495f))
        assertFalse(l.dpad.hit(310f, 770f))
    }

    @Test
    fun controlsDoNotOverlap() {
        val l = Layout()
        l.compute(2772f, 1272f, 1f, leftHanded = false)
        // D-pad above the left stick (same x column).
        assertTrue(
            l.dpad.cy + l.dpad.r <= l.leftStick.cy - l.leftStick.r + 0.5f,
            "d-pad and left stick overlap: ${l.dpad.cy + l.dpad.r} vs ${l.leftStick.cy - l.leftStick.r}",
        )
        // The Y button clears the right trigger rectangle.
        assertTrue(
            l.y.cy - l.y.r * 1.15f >= l.rt.bottom,
            "Y button overlaps the trigger rect: ${l.y.cy - l.y.r * 1.15f} vs ${l.rt.bottom}",
        )
        // A button misses the right stick (x ranges never meet).
        assertTrue(l.a.cx - l.a.r * 1.15f > l.rightStick.cx + l.rightStick.r)
    }

    @Test
    fun controlsStayAwayFromTheDisplayEdges() {
        val l = Layout()
        l.compute(2772f, 1272f, 1f, leftHanded = false)
        // Every control keeps a safe margin from the rounded corners and
        // gesture areas: nothing within 4% of an edge.
        val left = minOf(l.lt.left, l.lb.left, l.leftStick.cx - l.leftStick.r, l.dpad.cx - l.dpad.r)
        val right = maxOf(l.rt.right, l.rb.right, l.a.cx + l.a.r)
        val bottom = maxOf(l.leftStick.cy + l.leftStick.r, l.lsClick.cy + l.lsClick.r)
        assertTrue(left > 2772f * 0.04f, "left margin too small: $left")
        assertTrue(right < 2772f * 0.96f, "right margin too small")
        assertTrue(bottom < 1272f * 0.93f, "bottom margin too small: $bottom")
    }

    @Test
    fun leftHandedMirrorsHorizontally() {
        val l = Layout()
        l.compute(2000f, 1000f, 1f, leftHanded = true)
        assertEquals(1690f, l.leftStick.cx, 1f) // 2000 - 310
        assertEquals(310f, l.a.cx, 1f) // 2000 - 1690
    }

    @Test
    fun scaleGrowsAboutTheCentre() {
        val l = Layout()
        l.compute(2000f, 1000f, 1.2f, leftHanded = false)
        assertEquals(172f, l.leftStick.cx, 1f) // 1000 + (310 - 1000) * 1.2
        assertEquals(174f, l.leftStick.r, 1f) // 145 * 1.2
    }
}
