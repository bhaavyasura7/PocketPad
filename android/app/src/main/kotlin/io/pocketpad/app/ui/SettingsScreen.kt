package io.pocketpad.app.ui

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Slider
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import io.pocketpad.app.R
import io.pocketpad.app.core.Prefs
import kotlin.math.roundToInt

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(vm: AppViewModel) {
    var deadZone by remember { mutableIntStateOf(vm.prefs.deadZonePercent) }
    var analog by remember { mutableStateOf(vm.prefs.triggerAnalog) }
    var rate by remember { mutableIntStateOf(vm.prefs.rateHz) }
    var haptic by remember { mutableStateOf(vm.prefs.haptic) }
    var vibrate by remember { mutableStateOf(vm.prefs.vibrateOnRumble) }
    var scale by remember { mutableIntStateOf(vm.prefs.layoutScalePercent) }
    var leftHanded by remember { mutableStateOf(vm.prefs.leftHanded) }
    var doubleTap by remember { mutableStateOf(vm.prefs.stickDoubleTap) }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .statusBarsPadding()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 20.dp, vertical = 16.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(stringResource(R.string.settings_title), style = MaterialTheme.typography.headlineSmall)
            Spacer(Modifier.weight(1f))
            TextButton(onClick = vm::closeSettings) {
                Text(stringResource(R.string.done))
            }
        }

        Spacer(Modifier.height(8.dp))

        SettingSlider(
            label = stringResource(R.string.setting_dead_zone),
            valueLabel = stringResource(R.string.setting_dead_zone_value, deadZone),
            value = deadZone.toFloat(),
            range = 5f..30f,
            steps = 24,
        ) {
            deadZone = it.roundToInt()
            vm.prefs.deadZonePercent = deadZone
            vm.settingsChanged()
        }

        SettingSwitch(
            label = stringResource(R.string.setting_trigger_analog),
            summary = stringResource(R.string.setting_trigger_analog_summary),
            checked = analog,
        ) {
            analog = it
            vm.prefs.triggerAnalog = it
            vm.settingsChanged()
        }

        Text(stringResource(R.string.setting_rate), style = MaterialTheme.typography.bodyLarge)
        Spacer(Modifier.height(4.dp))
        SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
            Prefs.RATES.forEachIndexed { i, hz ->
                SegmentedButton(
                    selected = rate == hz,
                    onClick = {
                        rate = hz
                        vm.prefs.rateHz = hz
                        vm.settingsChanged()
                    },
                    shape = SegmentedButtonDefaults.itemShape(index = i, count = Prefs.RATES.size),
                ) { Text(stringResource(R.string.setting_rate_value, hz)) }
            }
        }
        Spacer(Modifier.height(12.dp))

        SettingSwitch(
            label = stringResource(R.string.setting_haptic),
            summary = null,
            checked = haptic,
        ) {
            haptic = it
            vm.prefs.haptic = it
            vm.settingsChanged()
        }

        SettingSwitch(
            label = stringResource(R.string.setting_rumble),
            summary = null,
            checked = vibrate,
        ) {
            vibrate = it
            vm.prefs.vibrateOnRumble = it
            vm.settingsChanged()
        }

        SettingSlider(
            label = stringResource(R.string.setting_scale),
            valueLabel = stringResource(R.string.setting_scale_value, scale),
            value = scale.toFloat(),
            range = 80f..120f,
            steps = 39,
        ) {
            scale = it.roundToInt()
            vm.prefs.layoutScalePercent = scale
            vm.settingsChanged()
        }

        SettingSwitch(
            label = stringResource(R.string.setting_lefthanded),
            summary = null,
            checked = leftHanded,
        ) {
            leftHanded = it
            vm.prefs.leftHanded = it
            vm.settingsChanged()
        }

        SettingSwitch(
            label = stringResource(R.string.setting_stick_double_tap),
            summary = stringResource(R.string.setting_stick_double_tap_summary),
            checked = doubleTap,
        ) {
            doubleTap = it
            vm.prefs.stickDoubleTap = it
            vm.settingsChanged()
        }

        Spacer(Modifier.height(16.dp))
        OutlinedButton(
            onClick = {
                vm.prefs.resetSettings()
                deadZone = vm.prefs.deadZonePercent
                analog = vm.prefs.triggerAnalog
                rate = vm.prefs.rateHz
                haptic = vm.prefs.haptic
                vibrate = vm.prefs.vibrateOnRumble
                scale = vm.prefs.layoutScalePercent
                leftHanded = vm.prefs.leftHanded
                doubleTap = vm.prefs.stickDoubleTap
                vm.settingsChanged()
            },
            modifier = Modifier.fillMaxWidth(),
        ) {
            Text(stringResource(R.string.reset_settings))
        }
    }
}

@Composable
private fun SettingSwitch(
    label: String,
    summary: String?,
    checked: Boolean,
    onChange: (Boolean) -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(label, style = MaterialTheme.typography.bodyLarge)
            if (summary != null) {
                Text(
                    summary,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        Switch(checked = checked, onCheckedChange = onChange)
    }
}

@Composable
private fun SettingSlider(
    label: String,
    valueLabel: String,
    value: Float,
    range: ClosedFloatingPointRange<Float>,
    steps: Int,
    onChange: (Float) -> Unit,
) {
    Column(modifier = Modifier.padding(vertical = 4.dp)) {
        Row {
            Text(label, style = MaterialTheme.typography.bodyLarge)
            Spacer(Modifier.weight(1f))
            Text(valueLabel, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        Slider(
            value = value,
            onValueChange = onChange,
            valueRange = range,
            steps = steps,
        )
    }
}
