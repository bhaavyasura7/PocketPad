package io.pocketpad.app.core

import java.net.URI
import java.net.URLDecoder

/**
 * Parses the QR code shown by `pocketpad app` on the computer:
 * `pocketpad://pair?h=<host>&p=<port>&c=<code>`. A lenient fallback also
 * accepts any text that contains an address and a pairing code, so a photo
 * of a code, a chat message or a printed label all work.
 */
object QrPairing {

    data class Pairing(val host: String, val port: Int, val code: String)

    fun parse(text: String): Pairing? {
        val trimmed = text.trim()
        primary(trimmed)?.let { return it }
        return fallback(trimmed)
    }

    private fun primary(text: String): Pairing? {
        val uri = try {
            URI(text)
        } catch (_: Exception) {
            return null
        }
        if (uri.scheme?.lowercase() != "pocketpad") return null
        val params = uri.query?.split('&')?.mapNotNull { part ->
            val i = part.indexOf('=')
            if (i <= 0) return@mapNotNull null
            val key = part.substring(0, i)
            val value = try {
                URLDecoder.decode(part.substring(i + 1), "UTF-8")
            } catch (_: Exception) {
                part.substring(i + 1)
            }
            key to value
        }?.toMap().orEmpty()
        val host = params["h"]?.takeIf { it.isNotBlank() } ?: return null
        val port = params["p"]?.toIntOrNull()?.takeIf { it in 1..65535 } ?: WirePort
        val code = params["c"]?.takeIf { it.isNotBlank() } ?: return null
        return Pairing(host, port, code)
    }

    // The canonical dashed form first, then an unbroken 24-character run.
    private val codePattern = Regex("[0-9A-Za-z]{4}(?:-[0-9A-Za-z]{4}){5}")
    private val plainCodePattern = Regex("[0-9A-Za-z]{24}")

    // Address-shaped only: an IPv4 literal, or a dotted hostname like
    // "mac.lan" — never a plain word.
    private val ipPattern = Regex("(\\d{1,3}(?:\\.\\d{1,3}){3})(?::(\\d{2,5}))?")
    private val hostPattern = Regex("([A-Za-z0-9][A-Za-z0-9-]*(?:\\.[A-Za-z0-9-]+)+)(?::(\\d{2,5}))?")

    private fun fallback(text: String): Pairing? {
        val code = codePattern.find(text)?.value
            ?: plainCodePattern.find(text)?.value
            ?: return null
        val address = ipPattern.find(text) ?: hostPattern.find(text) ?: return null
        val host = address.groupValues[1]
        val port = address.groupValues[2].toIntOrNull()?.takeIf { it in 1..65535 } ?: WirePort
        return Pairing(host, port, code)
    }

    private const val WirePort = io.pocketpad.protocol.Wire.DEFAULT_PORT
}
