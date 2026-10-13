package io.pocketpad.app

import io.pocketpad.app.core.RumbleDecision
import io.pocketpad.app.core.RumblePolicy
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class RumblePolicyTest {

    @Test
    fun disabledStops() {
        val d = RumblePolicy.decide(1000, 65535, 65535, 500, 0, amplitudeControl = true, enabled = false)
        assertTrue(d is RumbleDecision.Stop)
    }

    @Test
    fun zeroStrengthStops() {
        val d = RumblePolicy.decide(1000, 0, 0, 500, 0, amplitudeControl = true, enabled = true)
        assertTrue(d is RumbleDecision.Stop)
        val low = RumblePolicy.decide(1000, 100, 0, 500, 0, amplitudeControl = true, enabled = true)
        assertTrue(low is RumbleDecision.Stop) // below 0.02
    }

    @Test
    fun rapidUpdatesAreIgnored() {
        val d = RumblePolicy.decide(1000, 65535, 0, 500, 990, amplitudeControl = true, enabled = true)
        assertTrue(d is RumbleDecision.Ignore)
        val ok = RumblePolicy.decide(1015, 65535, 0, 500, 990, amplitudeControl = true, enabled = true)
        assertTrue(ok is RumbleDecision.Vibrate)
    }

    @Test
    fun strengthIsTheMaximumOfBothMotors() {
        val d = RumblePolicy.decide(1000, 0x4000, 0xFFFF, 250, 0, amplitudeControl = true, enabled = true)
        assertTrue(d is RumbleDecision.Vibrate)
        assertEquals(1f, d.strength, 0.001f)
        assertEquals(250, d.durationMs)
    }

    @Test
    fun withoutAmplitudeControlWeakRumbleIsIgnored() {
        val weak = RumblePolicy.decide(1000, 0x2000, 0, 500, 0, amplitudeControl = false, enabled = true)
        assertTrue(weak is RumbleDecision.Ignore)
        val strong = RumblePolicy.decide(1000, 0x8000, 0, 500, 0, amplitudeControl = false, enabled = true)
        assertTrue(strong is RumbleDecision.Vibrate)
    }

    @Test
    fun durationIsClampedToTheProtocolMaximum() {
        val d = RumblePolicy.decide(1000, 0xFFFF, 0, 60000, 0, amplitudeControl = true, enabled = true)
        assertTrue(d is RumbleDecision.Vibrate)
        assertEquals(2000, d.durationMs)
    }
}
