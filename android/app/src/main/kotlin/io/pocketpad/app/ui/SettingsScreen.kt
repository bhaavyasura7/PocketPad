package io.pocketpad.app.ui

import android.app.Activity
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
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
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
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
    var lights by remember { mutableStateOf(vm.prefs.gamingLights) }
    var lightsStyle by remember { mutableIntStateOf(vm.prefs.lightsStyle) }
    var lightsSpeed by remember { mutableIntStateOf(vm.prefs.lightsSpeed) }
    var lightsBrightness by remember { mutableIntStateOf(vm.prefs.lightsBrightness) }
    var lightsColor by remember { mutableIntStateOf(vm.prefs.lightsColor) }

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
            summary = stringResource(R.string.setting_haptic_summary),
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

        SettingSwitch(
            label = stringResource(R.string.setting_lights),
            summary = stringResource(R.string.setting_lights_summary),
            checked = lights,
        ) {
            lights = it
            vm.prefs.gamingLights = it
            vm.settingsChanged()
        }

        if (lights) {
            Text(stringResource(R.string.setting_lights_style), style = MaterialTheme.typography.bodyLarge)
            SegmentedRow(
                options = listOf(
                    stringResource(R.string.lights_rainbow) to 0,
                    stringResource(R.string.lights_solid) to 1,
                ),
                selected = lightsStyle,
            ) { lightsStyle = it; vm.prefs.lightsStyle = it; vm.settingsChanged() }

            Text(stringResource(R.string.setting_lights_speed), style = MaterialTheme.typography.bodyLarge)
            SegmentedRow(
                options = listOf(
                    stringResource(R.string.lights_slow) to 50,
                    stringResource(R.string.lights_normal) to 100,
                    stringResource(R.string.lights_fast) to 200,
                ),
                selected = lightsSpeed,
            ) { lightsSpeed = it; vm.prefs.lightsSpeed = it; vm.settingsChanged() }

            Text(stringResource(R.string.setting_lights_brightness), style = MaterialTheme.typography.bodyLarge)
            SegmentedRow(
                options = listOf(
                    stringResource(R.string.lights_soft) to 60,
                    stringResource(R.string.lights_normal) to 100,
                    stringResource(R.string.lights_bright) to 140,
                ),
                selected = lightsBrightness,
            ) { lightsBrightness = it; vm.prefs.lightsBrightness = it; vm.settingsChanged() }

            if (lightsStyle == 1) {
                Text(stringResource(R.string.setting_lights_color), style = MaterialTheme.typography.bodyLarge)
                Row(modifier = Modifier.padding(vertical = 8.dp)) {
                    LIGHT_COLORS.forEach { color ->
                        ColorDot(color = color, selected = lightsColor == color) {
                            lightsColor = color
                            vm.prefs.lightsColor = color
                            vm.settingsChanged()
                        }
                    }
                }
            }
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
                lights = vm.prefs.gamingLights
                lightsStyle = vm.prefs.lightsStyle
                lightsSpeed = vm.prefs.lightsSpeed
                lightsBrightness = vm.prefs.lightsBrightness
                lightsColor = vm.prefs.lightsColor
                vm.settingsChanged()
            },
            modifier = Modifier.fillMaxWidth(),
        ) {
            Text(stringResource(R.string.reset_settings))
        }

        val privacyRequired by Ads.privacyOptionsRequired.collectAsState()
        if (privacyRequired) {
            val activity = LocalContext.current as? Activity
            TextButton(
                onClick = { activity?.let(Ads::showPrivacyOptions) },
                modifier = Modifier.fillMaxWidth(),
            ) {
                Text(stringResource(R.string.privacy_options))
            }
        }

        Spacer(Modifier.height(16.dp))
        AdBanner()
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

private val LIGHT_COLORS = listOf(
    0xFF22D3EE.toInt(), // cyan
    0xFF4ADE80.toInt(), // green
    0xFFA78BFA.toInt(), // purple
    0xFFF472B6.toInt(), // pink
    0xFFF87171.toInt(), // red
    0xFFFBBF24.toInt(), // gold
    0xFFFFFFFF.toInt(), // white
)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SegmentedRow(options: List<Pair<String, Int>>, selected: Int, onSelect: (Int) -> Unit) {
    SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
        options.forEachIndexed { index, (label, value) ->
            SegmentedButton(
                selected = selected == value,
                onClick = { onSelect(value) },
                shape = SegmentedButtonDefaults.itemShape(index = index, count = options.size),
            ) { Text(label) }
        }
    }
    Spacer(Modifier.height(12.dp))
}

@Composable
private fun ColorDot(color: Int, selected: Boolean, onClick: () -> Unit) {
    Box(
        modifier = Modifier
            .padding(end = 10.dp)
            .size(34.dp)
            .clip(CircleShape)
            .background(Color(color))
            .border(
                width = if (selected) 3.dp else 1.dp,
                color = if (selected) MaterialTheme.colorScheme.onSurface else MaterialTheme.colorScheme.outline,
                shape = CircleShape,
            )
            .clickable(onClick = onClick),
    )
}
