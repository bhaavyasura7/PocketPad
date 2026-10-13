package io.pocketpad.app.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import io.pocketpad.app.R

private data class KeyRow(val button: String, val key: String, val what: String)

private val GTA_ROWS = listOf(
    KeyRow("X", "F", "enter / exit vehicle  ← the GTA question"),
    KeyRow("RT", "Left click", "fire / accelerate in a vehicle"),
    KeyRow("LT", "Right click", "aim / brake in a vehicle"),
    KeyRow("Left stick", "W A S D", "move"),
    KeyRow("Right stick", "Mouse", "look around"),
    KeyRow("A", "Space", "jump"),
    KeyRow("B", "Left Shift", "sprint"),
    KeyRow("Y", "Q", "take cover"),
    KeyRow("LB", "E", "interact / horn"),
    KeyRow("RB", "V", "change camera"),
    KeyRow("D-pad", "Arrow keys", "menus / radio"),
    KeyRow("Back", "M", "interaction menu"),
    KeyRow("Start", "Esc", "pause menu"),
    KeyRow("Guide", "Tab", "player list"),
    KeyRow("LS / RS", "C / R", "look behind / radio"),
)

private val RACING_ROWS = listOf(
    KeyRow("RT", "W", "accelerate"),
    KeyRow("LT", "S", "brake / reverse"),
    KeyRow("Left stick", "A / D", "steer"),
    KeyRow("A", "Space", "handbrake"),
    KeyRow("X", "C", "change camera"),
    KeyRow("Y", "R", "reset car"),
    KeyRow("LB / RB", "Q / E", "look left / right"),
    KeyRow("D-pad", "Arrow keys", "menus"),
    KeyRow("Back", "M", "map"),
    KeyRow("Start", "Enter", "pause / accept"),
    KeyRow("Guide", "Esc", "menu"),
    KeyRow("LS", "H", "horn"),
    KeyRow("RS", "V", "camera toggle"),
)

private val GENERAL_ROWS = listOf(
    KeyRow("Left stick", "W A S D", "move / walk"),
    KeyRow("Right stick", "Mouse", "look / pointer"),
    KeyRow("LT / RT", "Click", "left / right mouse button"),
    KeyRow("A", "Space", "jump"),
    KeyRow("B", "Left Shift", "sneak"),
    KeyRow("X", "E", "use / inventory"),
    KeyRow("Y", "Q", "drop"),
    KeyRow("LB", "Tab", "list"),
    KeyRow("RB", "Esc", "menu"),
    KeyRow("Back", "M", "map"),
    KeyRow("Start", "Enter", "chat / confirm"),
    KeyRow("Guide", "F1", "—"),
    KeyRow("LS / RS", "C / R", "zoom / —"),
    KeyRow("D-pad", "Arrow keys", "menus"),
)

@Composable
fun ControlsScreen(vm: AppViewModel) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .statusBarsPadding()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 20.dp, vertical = 16.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(stringResource(R.string.controls_title), style = MaterialTheme.typography.headlineSmall)
            Spacer(Modifier.weight(1f))
            TextButton(onClick = vm::closeControls) {
                Text(stringResource(R.string.done))
            }
        }
        Text(
            stringResource(R.string.controls_profile_note),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        ProfileSection(stringResource(R.string.controls_gta), GTA_ROWS)
        ProfileSection(stringResource(R.string.controls_racing), RACING_ROWS)
        ProfileSection(stringResource(R.string.controls_general), GENERAL_ROWS)
    }
}

@Composable
private fun ProfileSection(title: String, rows: List<KeyRow>) {
    Spacer(Modifier.height(18.dp))
    Text(title, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
    Spacer(Modifier.height(6.dp))
    rows.forEach { row ->
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(vertical = 3.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            KeyCap(row.button)
            Spacer(Modifier.width(10.dp))
            Text(
                row.key,
                style = MaterialTheme.typography.bodyMedium,
                fontFamily = FontFamily.Monospace,
                modifier = Modifier.width(110.dp),
            )
            Text(
                row.what,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.weight(1f),
            )
        }
    }
}

@Composable
private fun KeyCap(label: String) {
    Box(
        modifier = Modifier
            .background(MaterialTheme.colorScheme.surfaceVariant, RoundedCornerShape(6.dp))
            .padding(horizontal = 8.dp, vertical = 4.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            label,
            style = MaterialTheme.typography.labelMedium,
            fontFamily = FontFamily.Monospace,
        )
    }
}
