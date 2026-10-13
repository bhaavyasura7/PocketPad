package io.pocketpad.app.ui

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

private val Accent = Color(0xFF7C5CFF)

private val Colors = darkColorScheme(
    primary = Accent,
    onPrimary = Color.White,
    primaryContainer = Color(0xFF2A2350),
    onPrimaryContainer = Color(0xFFE3DCFF),
    secondary = Color(0xFF9E86FF),
    background = Color(0xFF0B0D12),
    onBackground = Color(0xFFECEAF4),
    surface = Color(0xFF12151D),
    onSurface = Color(0xFFECEAF4),
    surfaceVariant = Color(0xFF1B1F2A),
    onSurfaceVariant = Color(0xFFB9B7C5),
    error = Color(0xFFFF6E6E),
    onError = Color(0xFF2A0505),
    outline = Color(0xFF4A4E5C),
)

@Composable
fun PocketPadTheme(content: @Composable () -> Unit) {
    MaterialTheme(colorScheme = Colors, content = content)
}
