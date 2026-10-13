package io.pocketpad.app.ui

import android.Manifest
import android.bluetooth.BluetoothAdapter
import android.bluetooth.BluetoothDevice
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.Image
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.unit.dp
import com.journeyapps.barcodescanner.ScanContract
import com.journeyapps.barcodescanner.ScanOptions
import io.pocketpad.app.R
import io.pocketpad.app.core.Discovery
import io.pocketpad.app.core.QrPairing
import io.pocketpad.protocol.Pairing
import io.pocketpad.protocol.Wire
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

private const val MODE_WIFI = 0
private const val MODE_BLUETOOTH = 1

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ConnectScreen(vm: AppViewModel) {
    var mode by rememberSaveable { mutableIntStateOf(MODE_WIFI) }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .statusBarsPadding()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 20.dp, vertical = 16.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Image(
                painter = painterResource(R.drawable.ic_launcher_logo),
                contentDescription = null,
                modifier = Modifier
                    .size(52.dp)
                    .clip(RoundedCornerShape(14.dp)),
            )
            Spacer(Modifier.width(14.dp))
            Column(modifier = Modifier.weight(1f)) {
                Text(stringResource(R.string.connect_title), style = MaterialTheme.typography.headlineMedium)
                Text(
                    stringResource(R.string.connect_subtitle),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            IconButton(onClick = vm::openSettings) {
                Icon(Icons.Filled.Settings, contentDescription = stringResource(R.string.settings))
            }
        }

        Spacer(Modifier.height(16.dp))

        SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
            SegmentedButton(
                selected = mode == MODE_WIFI,
                onClick = { mode = MODE_WIFI },
                shape = SegmentedButtonDefaults.itemShape(index = 0, count = 2),
            ) { Text(stringResource(R.string.mode_wifi)) }
            SegmentedButton(
                selected = mode == MODE_BLUETOOTH,
                onClick = { mode = MODE_BLUETOOTH },
                shape = SegmentedButtonDefaults.itemShape(index = 1, count = 2),
            ) { Text(stringResource(R.string.mode_bluetooth)) }
        }

        Spacer(Modifier.height(16.dp))

        if (mode == MODE_WIFI) WifiPanel(vm) else BluetoothPanel(vm)
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun WifiPanel(vm: AppViewModel) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()

    var scanning by remember { mutableStateOf(false) }
    var found by remember { mutableStateOf<List<Discovery.Found>>(emptyList()) }
    var host by rememberSaveable { mutableStateOf("") }
    var portText by rememberSaveable { mutableStateOf(Wire.DEFAULT_PORT.toString()) }
    var code by rememberSaveable(stateSaver = TextFieldValue.Saver) {
        mutableStateOf(TextFieldValue(""))
    }
    var selectedName by rememberSaveable { mutableStateOf("") }
    var error by remember { mutableStateOf<String?>(null) }
    var savedVersion by remember { mutableIntStateOf(0) }
    val saved = remember(savedVersion) { vm.prefs.receivers() }

    fun scan() {
        scope.launch {
            scanning = true
            found = withContext(Dispatchers.IO) { Discovery.scan(1000) }
            scanning = false
        }
    }

    LaunchedEffect(Unit) { scan() }

    fun connect() {
        error = null
        val h = host.trim()
        if (h.isEmpty()) {
            error = context.getString(R.string.connect_error_address)
            return
        }
        val port = portText.trim().toIntOrNull()?.takeIf { it in 1..65535 } ?: Wire.DEFAULT_PORT
        val savedSecret = vm.prefs.secretFor("$h:$port")
        val secret = if (code.text.isNotBlank()) Pairing.parse(code.text) else savedSecret
        if (secret == null) {
            error = context.getString(R.string.connect_error_code)
            return
        }
        vm.connectWifi(h, port, secret, selectedName.ifBlank { h })
    }

    val scanQr = rememberLauncherForActivityResult(ScanContract()) { result ->
        val contents = result.contents
        if (contents != null) {
            val p = QrPairing.parse(contents)
            if (p != null) {
                host = p.host
                portText = p.port.toString()
                code = formatPairingInput(TextFieldValue(p.code))
                selectedName = ""
                error = null
            } else {
                error = context.getString(R.string.qr_invalid)
            }
        }
    }

    Row(verticalAlignment = Alignment.CenterVertically) {
        FilledTonalButton(onClick = ::scan, enabled = !scanning) {
            Icon(Icons.Filled.Refresh, contentDescription = null)
            Spacer(Modifier.width(8.dp))
            Text(stringResource(if (scanning) R.string.scanning else R.string.scan))
        }
        Spacer(Modifier.width(8.dp))
        OutlinedButton(onClick = {
            scanQr.launch(
                ScanOptions()
                    .setDesiredBarcodeFormats(ScanOptions.QR_CODE)
                    .setPrompt(context.getString(R.string.scan_qr_prompt))
                    .setBeepEnabled(false)
                    .setOrientationLocked(true),
            )
        }) {
            Text(stringResource(R.string.scan_qr))
        }
    }

    if (found.isNotEmpty()) {
        Spacer(Modifier.height(8.dp))
        Text(stringResource(R.string.receivers_found), style = MaterialTheme.typography.titleSmall)
        found.forEach { f ->
            ListItem(
                headlineContent = { Text(f.name.ifBlank { f.host }) },
                supportingContent = { Text("${f.host}:${f.port}") },
                modifier = Modifier.clickable {
                    host = f.host
                    portText = f.port.toString()
                    selectedName = f.name
                },
            )
        }
    } else if (!scanning) {
        Text(
            stringResource(R.string.no_receivers),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(vertical = 8.dp),
        )
    }

    Spacer(Modifier.height(16.dp))
    Text(stringResource(R.string.manual_address), style = MaterialTheme.typography.titleSmall)
    Spacer(Modifier.height(8.dp))

    Row(verticalAlignment = Alignment.CenterVertically) {
        OutlinedTextField(
            value = host,
            onValueChange = { host = it.trim() },
            label = { Text(stringResource(R.string.manual_address)) },
            placeholder = { Text(stringResource(R.string.manual_address_hint)) },
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
            modifier = Modifier.weight(1f),
        )
        Spacer(Modifier.width(8.dp))
        OutlinedTextField(
            value = portText,
            onValueChange = { portText = it.filter(Char::isDigit).take(5) },
            label = { Text(stringResource(R.string.port_label)) },
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
            modifier = Modifier.width(96.dp),
        )
    }

    Spacer(Modifier.height(8.dp))

    OutlinedTextField(
        value = code,
        onValueChange = { raw -> code = formatPairingInput(raw) },
        label = { Text(stringResource(R.string.pairing_code)) },
        placeholder = { Text(stringResource(R.string.pairing_code_hint)) },
        singleLine = true,
        keyboardOptions = KeyboardOptions(
            keyboardType = KeyboardType.Ascii,
            capitalization = KeyboardCapitalization.Characters,
        ),
        modifier = Modifier.fillMaxWidth(),
    )

    error?.let {
        Spacer(Modifier.height(8.dp))
        Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
    }

    Spacer(Modifier.height(12.dp))
    Button(onClick = ::connect, modifier = Modifier.fillMaxWidth()) {
        Text(stringResource(R.string.connect))
    }

    if (saved.isNotEmpty()) {
        Spacer(Modifier.height(20.dp))
        Text(stringResource(R.string.saved_receivers), style = MaterialTheme.typography.titleSmall)
        saved.forEach { r ->
            ListItem(
                headlineContent = { Text(r.name.ifBlank { r.key }) },
                supportingContent = { Text(r.key) },
                trailingContent = {
                    Row {
                        FilledTonalButton(onClick = {
                            val parts = r.key.split(':')
                            vm.connectWifi(
                                parts[0],
                                parts.getOrNull(1)?.toIntOrNull() ?: Wire.DEFAULT_PORT,
                                r.secret,
                                r.name,
                            )
                        }) { Text(stringResource(R.string.connect)) }
                        Spacer(Modifier.width(8.dp))
                        OutlinedButton(onClick = {
                            vm.prefs.forgetReceiver(r.key)
                            savedVersion++
                        }) { Text(stringResource(R.string.forget)) }
                    }
                },
                modifier = Modifier.clickable {
                    val parts = r.key.split(':')
                    host = parts[0]
                    portText = parts.getOrNull(1) ?: Wire.DEFAULT_PORT.toString()
                    selectedName = r.name
                },
            )
        }
    }
}

@Composable
private fun BluetoothPanel(vm: AppViewModel) {
    val context = LocalContext.current
    val bt = remember { vm.bluetooth() }
    var granted by remember { mutableStateOf(btPermissionsGranted(context)) }
    var hostsVersion by remember { mutableIntStateOf(0) }
    val hosts = remember(hostsVersion) { bt.pairedHosts() }

    val permissionLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestMultiplePermissions(),
    ) { granted = btPermissionsGranted(context) }
    val discoverableLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.StartActivityForResult(),
    ) { }

    if (!bt.available) {
        Text(
            stringResource(R.string.error_bt_unavailable),
            color = MaterialTheme.colorScheme.error,
        )
        return
    }

    Text(
        stringResource(R.string.bt_explainer),
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    Spacer(Modifier.height(12.dp))
    Text(
        stringResource(R.string.bt_rumble_unsupported),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    Spacer(Modifier.height(16.dp))

    if (!granted) {
        Button(onClick = {
            if (Build.VERSION.SDK_INT >= 31) {
                permissionLauncher.launch(
                    arrayOf(Manifest.permission.BLUETOOTH_CONNECT, Manifest.permission.BLUETOOTH_ADVERTISE),
                )
            } else {
                granted = true
            }
        }) {
            Text(stringResource(R.string.bt_grant_permissions))
        }
    } else {
        OutlinedButton(onClick = {
            discoverableLauncher.launch(
                Intent(BluetoothAdapter.ACTION_REQUEST_DISCOVERABLE)
                    .putExtra(BluetoothAdapter.EXTRA_DISCOVERABLE_DURATION, 120),
            )
        }) {
            Text(stringResource(R.string.bt_make_discoverable))
        }
        Spacer(Modifier.height(4.dp))
        Text(
            stringResource(R.string.bt_discoverable_hint),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        Spacer(Modifier.height(16.dp))
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(stringResource(R.string.bt_paired_computers), style = MaterialTheme.typography.titleSmall)
            Spacer(Modifier.weight(1f))
            IconButton(onClick = { hostsVersion++ }) {
                Icon(Icons.Filled.Refresh, contentDescription = stringResource(R.string.scan))
            }
        }
        if (hosts.isEmpty()) {
            Text(
                stringResource(R.string.bt_none_paired),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        } else {
            hosts.forEach { d ->
                val name = deviceName(d)
                ListItem(
                    headlineContent = { Text(name) },
                    supportingContent = { Text(d.address) },
                    trailingContent = {
                        FilledTonalButton(onClick = { vm.connectBluetooth(d) }) {
                            Text(stringResource(R.string.connect))
                        }
                    },
                    modifier = Modifier.clickable { vm.connectBluetooth(d) },
                )
            }
        }
    }
}

private fun btPermissionsGranted(context: Context): Boolean {
    if (Build.VERSION.SDK_INT < 31) return true
    return context.checkSelfPermission(Manifest.permission.BLUETOOTH_CONNECT) == PackageManager.PERMISSION_GRANTED &&
        context.checkSelfPermission(Manifest.permission.BLUETOOTH_ADVERTISE) == PackageManager.PERMISSION_GRANTED
}

private fun deviceName(d: BluetoothDevice): String = try {
    d.name ?: d.address
} catch (_: SecurityException) {
    d.address
}

/**
 * Auto-uppercases and inserts dashes as the user types a pairing code.
 * When the text is rewritten the cursor must land at the end; otherwise the
 * next keystroke is inserted mid-string and the code gets scrambled.
 */
private fun formatPairingInput(raw: TextFieldValue): TextFieldValue {
    val clean = raw.text.uppercase().filter { it.isLetterOrDigit() }.take(24)
    val formatted = clean.chunked(4).joinToString("-")
    if (formatted == raw.text) return raw
    return TextFieldValue(formatted, TextRange(formatted.length))
}
