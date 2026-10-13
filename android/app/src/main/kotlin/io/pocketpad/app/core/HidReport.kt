package io.pocketpad.app.core

import io.pocketpad.protocol.Codec
import io.pocketpad.protocol.ControllerState
import io.pocketpad.protocol.DPad

/**
 * The Bluetooth-HID report used in Bluetooth mode: report ID 1, 13 data
 * bytes. This is the descriptor and mapping specified in docs/HANDOFF.md
 * §4.4; a known state must produce a known report (unit tested).
 */
object HidReport {
    const val REPORT_ID = 1
    const val DATA_SIZE = 13

    /** Report descriptor: 16 buttons, hat switch, 4 int16 axes, 2 uint8 triggers. */
    val DESCRIPTOR: ByteArray = byteArrayOf(
        0x05, 0x01,             // Usage Page (Generic Desktop)
        0x09, 0x05,             // Usage (Game Pad)
        0xA1.toByte(), 0x01,    // Collection (Application)
        0x85.toByte(), 0x01,    //   Report ID (1)
        0x05, 0x09,             //   Usage Page (Button)
        0x19, 0x01, 0x29, 0x10, //   Usage Min 1, Max 16
        0x15, 0x00, 0x25, 0x01, //   Logical 0..1
        0x75, 0x01, 0x95.toByte(), 0x10, //   16 x 1 bit
        0x81.toByte(), 0x02,    //   Input (Data,Var,Abs)       -> bytes 0-1: buttons
        0x05, 0x01,             //   Usage Page (Generic Desktop)
        0x09, 0x39,             //   Usage (Hat switch)
        0x15, 0x00, 0x25, 0x07, //   Logical 0..7
        0x35, 0x00, 0x46, 0x3B, 0x01, // Physical 0..315
        0x65, 0x14,             //   Unit (Degrees)
        0x75, 0x04, 0x95.toByte(), 0x01,
        0x81.toByte(), 0x42,    //   Input (Data,Var,Abs,Null)  -> byte 2 low nibble
        0x65, 0x00,             //   Unit (None)
        0x35, 0x00, 0x45, 0x00, // Physical reset
        0x75, 0x04, 0x95.toByte(), 0x01,
        0x81.toByte(), 0x03,    //   Input (Const)              -> byte 2 high nibble
        0x09, 0x30, 0x09, 0x31, 0x09, 0x32, 0x09, 0x35, // X, Y, Z, Rz
        0x16, 0x01, 0x80.toByte(), 0x26, 0xFF.toByte(), 0x7F, // Logical -32767..32767
        0x75, 0x10, 0x95.toByte(), 0x04,
        0x81.toByte(), 0x02,    //   Input                      -> bytes 3-10: LX, LY, RX, RY
        0x05, 0x02,             //   Usage Page (Simulation Controls)
        0x09, 0xC5.toByte(), 0x09, 0xC4.toByte(), // Brake (LT), Accelerator (RT)
        0x15, 0x00, 0x26, 0xFF.toByte(), 0x00,    // Logical 0..255
        0x75, 0x08, 0x95.toByte(), 0x02,
        0x81.toByte(), 0x02,    //   Input                      -> bytes 11-12: LT, RT
        0xC0.toByte(),          // End Collection
    )

    /**
     * Encodes [s] into [out] (must hold [DATA_SIZE] bytes). Button bits map
     * 1:1 to the protocol (HID button 1 = A … 11 = Guide); the hat switch is
     * 0..7 with 8 meaning null; HID Y grows downwards, so LY and RY are
     * negated.
     */
    fun encode(s: ControllerState, out: ByteArray) {
        require(out.size >= DATA_SIZE) { "report buffer too small" }
        val buttons = s.buttons and 0x07FF
        out[0] = buttons.toByte()
        out[1] = (buttons shr 8).toByte()
        out[2] = (if (s.dpad == DPad.NEUTRAL) 8 else s.dpad - 1).toByte()
        Codec.put16(out, 3, Codec.encodeAxis(s.lx))
        Codec.put16(out, 5, -Codec.encodeAxis(s.ly))
        Codec.put16(out, 7, Codec.encodeAxis(s.rx))
        Codec.put16(out, 9, -Codec.encodeAxis(s.ry))
        out[11] = Codec.encodeTrigger(s.lt).toByte()
        out[12] = Codec.encodeTrigger(s.rt).toByte()
    }
}
