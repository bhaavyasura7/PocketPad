package io.pocketpad.app

import io.pocketpad.app.core.QrPairing
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class QrPairingTest {

    @Test
    fun parsesTheAppUri() {
        val p = QrPairing.parse("pocketpad://pair?h=192.168.31.85&p=47770&c=AVBB-QGJC-GNVM-T9DT-G7EM-VKR7")
        assertEquals("192.168.31.85", p?.host)
        assertEquals(47770, p?.port)
        assertEquals("AVBB-QGJC-GNVM-T9DT-G7EM-VKR7", p?.code)
    }

    @Test
    fun portDefaultsToTheProtocolPort() {
        val p = QrPairing.parse("pocketpad://pair?h=mac.lan&c=AVBB-QGJC-GNVM-T9DT-G7EM-VKR7")
        assertEquals(47770, p?.port)
        assertEquals("mac.lan", p?.host)
    }

    @Test
    fun rejectsUnrelatedQrCodes() {
        assertNull(QrPairing.parse("https://example.com"))
        assertNull(QrPairing.parse("WIFI:S:MyNetwork;T:WPA;P:secret;;"))
        assertNull(QrPairing.parse("AVBB-QGJC-GNVM-T9DT-G7EM-VKR7")) // code alone, no address
    }

    @Test
    fun fallbackAcceptsPlainTextWithAddressAndCode() {
        val p = QrPairing.parse("Receiver at 192.168.31.85:47770\ncode AVBB-QGJC-GNVM-T9DT-G7EM-VKR7")
        assertEquals("192.168.31.85", p?.host)
        assertEquals(47770, p?.port)
        assertEquals("AVBB-QGJC-GNVM-T9DT-G7EM-VKR7", p?.code)
    }
}
