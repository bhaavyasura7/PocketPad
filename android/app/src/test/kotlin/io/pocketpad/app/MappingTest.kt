package io.pocketpad.app

import io.pocketpad.app.core.Mapping
import io.pocketpad.protocol.DPad
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotEquals
import kotlin.test.assertNull

class MappingTest {

    @Test
    fun deadZoneInsideReturnsNull() {
        assertNull(Mapping.deadZone(0f, 0f, 0.1f))
        assertNull(Mapping.deadZone(0.05f, 0.05f, 0.1f))
    }

    @Test
    fun deadZoneRescales() {
        val v = Mapping.deadZone(0.5f, 0f, 0.1f)!!
        assertEquals(0.4444f, v[0], 0.001f)
        assertEquals(0f, v[1], 0.001f)
    }

    @Test
    fun deadZoneClampsToUnitCircle() {
        val v = Mapping.deadZone(3f, 4f, 0.1f)!!
        assertEquals(0.6f, v[0], 0.001f)
        assertEquals(0.8f, v[1], 0.001f)
    }

    @Test
    fun triggerCurve() {
        // Top 10%: about 0.15; bottom 40%: 1.0; linear between.
        assertEquals(0.15f, Mapping.triggerValue(0f, true), 0.001f)
        assertEquals(0.15f, Mapping.triggerValue(0.1f, true), 0.001f)
        assertEquals(0.575f, Mapping.triggerValue(0.35f, true), 0.001f)
        assertEquals(1f, Mapping.triggerValue(0.6f, true), 0.001f)
        assertEquals(1f, Mapping.triggerValue(1f, true), 0.001f)
    }

    @Test
    fun triggerFullPressMode() {
        assertEquals(1f, Mapping.triggerValue(0f, false), 0.001f)
        assertEquals(1f, Mapping.triggerValue(0.3f, false), 0.001f)
    }

    @Test
    fun dpadSectors() {
        val r = 100f
        assertEquals(DPad.UP, Mapping.dpadSector(0f, -r, r))
        assertEquals(DPad.UP_RIGHT, Mapping.dpadSector(r, -r, r))
        assertEquals(DPad.RIGHT, Mapping.dpadSector(r, 0f, r))
        assertEquals(DPad.DOWN_RIGHT, Mapping.dpadSector(r, r, r))
        assertEquals(DPad.DOWN, Mapping.dpadSector(0f, r, r))
        assertEquals(DPad.DOWN_LEFT, Mapping.dpadSector(-r, r, r))
        assertEquals(DPad.LEFT, Mapping.dpadSector(-r, 0f, r))
        assertEquals(DPad.UP_LEFT, Mapping.dpadSector(-r, -r, r))
    }

    @Test
    fun dpadCentreIsNeutral() {
        assertEquals(DPad.NEUTRAL, Mapping.dpadSector(0f, 0f, 100f))
        assertEquals(DPad.NEUTRAL, Mapping.dpadSector(20f, 0f, 100f))
        assertNotEquals(DPad.NEUTRAL, Mapping.dpadSector(30f, 0f, 100f))
    }
}
