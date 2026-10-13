package io.pocketpad.app

import io.pocketpad.app.core.HidReport
import io.pocketpad.protocol.Buttons
import io.pocketpad.protocol.ControllerState
import io.pocketpad.protocol.DPad
import kotlin.test.Test
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals

class HidReportTest {

    @Test
    fun neutralReport() {
        val out = ByteArray(HidReport.DATA_SIZE)
        HidReport.encode(ControllerState(), out)
        assertContentEquals(
            byteArrayOf(0, 0, 8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0),
            out,
            "neutral: buttons clear, hat null (8), axes centred, triggers zero",
        )
    }

    @Test
    fun knownStateProducesKnownBytes() {
        val s = ControllerState().apply {
            lx = 1f
            ly = 1f // protocol +Y up -> HID Y must be negated
            rx = -1f
            ry = -1f
            lt = 1f
            rt = 0f
            buttons = Buttons.A or Buttons.START
            dpad = DPad.UP
        }
        val out = ByteArray(HidReport.DATA_SIZE)
        HidReport.encode(s, out)
        assertContentEquals(
            byteArrayOf(
                0x81.toByte(), 0x00, // A (bit 0) + Start (bit 7)
                0x00, // hat: dpad UP (1) -> 0
                0xFF.toByte(), 0x7F, // LX = +32767
                0x01, 0x80.toByte(), // LY = -32767
                0x01, 0x80.toByte(), // RX = -32767
                0xFF.toByte(), 0x7F, // RY = +32767
                0xFF.toByte(), // LT full
                0x00, // RT off
            ),
            out,
        )
    }

    @Test
    fun allButtonsFitInTwoBytesWithReservedBitsCleared() {
        val s = ControllerState().apply { buttons = 0xFFFF }
        val out = ByteArray(HidReport.DATA_SIZE)
        HidReport.encode(s, out)
        // Only the 11 defined bits are sent.
        assertEquals((Buttons.DEFINED and 0xFF).toByte(), out[0])
        assertEquals(((Buttons.DEFINED shr 8) and 0xFF).toByte(), out[1])
    }

    @Test
    fun descriptorIsStable() {
        // The descriptor is frozen for Bluetooth mode; changing it is a
        // protocol-visible change (docs/HANDOFF.md §4.4).
        assertEquals(0x05, HidReport.DESCRIPTOR[0])
        assertEquals(0xC0.toByte(), HidReport.DESCRIPTOR.last())
        assertEquals(95, HidReport.DESCRIPTOR.size)
    }
}
