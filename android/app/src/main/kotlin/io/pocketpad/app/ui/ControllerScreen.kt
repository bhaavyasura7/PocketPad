package io.pocketpad.app.ui

import android.app.Activity
import android.view.WindowManager
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Menu
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.pocketpad.app.R
import io.pocketpad.app.core.BluetoothHidTransport
import io.pocketpad.app.core.ControllerView
import io.pocketpad.app.core.ErrorKind
import io.pocketpad.app.core.Haptics
import io.pocketpad.app.core.Stats
import io.pocketpad.app.core.Transport
import io.pocketpad.app.core.TransportStatus
import io.pocketpad.app.core.WifiTransport
import io.pocketpad.protocol.Wire
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow

private val Green = android.graphics.Color.parseColor("#63D471")
private val Amber = android.graphics.Color.parseColor("#FFB74D")
private val Red = android.graphics.Color.parseColor("#EF5350")
private val Gray = android.graphics.Color.parseColor("#6B6F7B")

@Composable
fun ControllerScreen(vm: AppViewModel) {
    val context = LocalContext.current
    val activity = context as? Activity
    val transport = vm.transport

    val idleStatus = remember { MutableStateFlow<TransportStatus>(TransportStatus.Idle) }
    val idleStats = remember { MutableStateFlow(Stats()) }
    val status by (transport?.status ?: idleStatus).collectAsStateWithLifecycle()
    val stats by (transport?.stats ?: idleStats).collectAsStateWithLifecycle()

    var battery by remember { mutableStateOf(vm.battery.read()) }
    LaunchedEffect(Unit) {
        while (true) {
            delay(5000)
            battery = vm.battery.read()
        }
    }

    // Full screen: keep the screen on and hide the system bars.
    DisposableEffect(activity) {
        val window = activity?.window
        window?.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        val controller = window?.let { WindowInsetsControllerCompat(it, it.decorView) }
        controller?.systemBarsBehavior =
            WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
        controller?.hide(WindowInsetsCompat.Type.systemBars())
        onDispose {
            window?.clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
            controller?.show(WindowInsetsCompat.Type.systemBars())
        }
    }

    // Leaving the foreground releases everything and ends the Wi-Fi session;
    // returning reconnects automatically. A Bluetooth HID link is kept.
    val lifecycleOwner = LocalLifecycleOwner.current
    DisposableEffect(lifecycleOwner, transport) {
        val observer = LifecycleEventObserver { _, event ->
            when (event) {
                Lifecycle.Event.ON_STOP -> {
                    vm.input.clear()
                    if (transport is WifiTransport) transport.stop()
                }
                Lifecycle.Event.ON_START -> {
                    if (transport is WifiTransport) transport.start()
                }
                else -> Unit
            }
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
    }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(MaterialTheme.colorScheme.background),
    ) {
        val haptics = remember(context) { Haptics(context) { vm.prefs.haptic } }
        AndroidView(
            factory = { ctx ->
                ControllerView(ctx).apply {
                    input = vm.input
                    this.haptics = haptics
                    settingsProvider = { vm.uiSettings() }
                }
            },
            update = { v ->
                v.input = vm.input
                v.haptics = haptics
                v.applySettings(vm.uiSettings())
            },
            modifier = Modifier.fillMaxSize(),
        )

        // The overlay lives in the top-centre gap between the bumper columns:
        // from 0.19w to 0.81w nothing else is drawn, so it never covers a
        // control on edge-to-edge phones.
        BoxWithConstraints(
            modifier = Modifier
                .fillMaxWidth()
                .systemBarsPadding(),
        ) {
            val sideInset = maxWidth * 0.19f
            Row(
                modifier = Modifier
                    .align(Alignment.TopStart)
                    .padding(start = sideInset, top = 6.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Box(
                    modifier = Modifier
                        .size(10.dp)
                        .background(statusColor(status), CircleShape),
                )
                Spacer(Modifier.width(8.dp))
                Column {
                    Text(statusText(transport, status), style = MaterialTheme.typography.bodySmall)
                    if (status is TransportStatus.Connected) {
                        val rttMs = (stats.rttUs + 500) / 1000
                        Text(
                            stringResource(R.string.rtt_label, rttMs) + " · " +
                                stringResource(R.string.rate_label, stats.rateHz),
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
            Row(
                modifier = Modifier
                    .align(Alignment.TopEnd)
                    .padding(end = sideInset, top = 6.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                if (battery.percent in 0..100) {
                    Text(
                        stringResource(R.string.battery_label, battery.percent) + if (battery.charging) " ⚡" else "",
                        style = MaterialTheme.typography.bodySmall,
                    )
                    Spacer(Modifier.width(4.dp))
                }
                MenuButton(vm)
            }
        }

        if (status is TransportStatus.Error) {
            // Scrim so stray touches do not reach the pad behind the card.
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .pointerInput(Unit) { detectTapGestures { } },
            )
            Card(
                modifier = Modifier
                    .align(Alignment.Center)
                    .padding(24.dp),
                colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceVariant),
            ) {
                Column(
                    modifier = Modifier.padding(20.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    Text(errorText(status as TransportStatus.Error), style = MaterialTheme.typography.bodyMedium)
                    Spacer(Modifier.height(16.dp))
                    Button(onClick = vm::exitController) {
                        Text(stringResource(R.string.back_to_connect))
                    }
                }
            }
        }
    }
}

@Composable
private fun MenuButton(vm: AppViewModel) {
    var open by remember { mutableStateOf(false) }
    Box {
        IconButton(onClick = { open = true }) {
            Icon(Icons.Filled.Menu, contentDescription = stringResource(R.string.menu))
        }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            DropdownMenuItem(
                text = { Text(stringResource(R.string.controls_title)) },
                onClick = {
                    open = false
                    vm.openControls()
                },
            )
            DropdownMenuItem(
                text = { Text(stringResource(R.string.settings)) },
                onClick = {
                    open = false
                    vm.openSettings()
                },
            )
            DropdownMenuItem(
                text = { Text(stringResource(R.string.back_to_connect)) },
                onClick = {
                    open = false
                    vm.exitController()
                },
            )
        }
    }
}

private fun statusColor(status: TransportStatus): androidx.compose.ui.graphics.Color = when (status) {
    is TransportStatus.Connected -> androidx.compose.ui.graphics.Color(Green)
    is TransportStatus.Connecting, is TransportStatus.Reconnecting -> androidx.compose.ui.graphics.Color(Amber)
    is TransportStatus.Error -> androidx.compose.ui.graphics.Color(Red)
    TransportStatus.Idle -> androidx.compose.ui.graphics.Color(Gray)
}

@Composable
private fun statusText(transport: Transport?, status: TransportStatus): String = when (status) {
    TransportStatus.Idle -> stringResource(R.string.status_idle)
    is TransportStatus.Connecting ->
        if (transport is BluetoothHidTransport) stringResource(R.string.status_waiting_host)
        else stringResource(R.string.status_connecting)
    is TransportStatus.Connected -> stringResource(R.string.status_connected)
    is TransportStatus.Reconnecting -> stringResource(R.string.status_reconnecting)
    is TransportStatus.Error -> errorText(status)
}

@Composable
private fun errorText(e: TransportStatus.Error): String = when (e.kind) {
    ErrorKind.NO_ANSWER -> stringResource(R.string.error_no_answer)
    ErrorKind.REJECT -> when (e.reason) {
        Wire.REJECT_BUSY -> stringResource(R.string.error_reject_busy)
        Wire.REJECT_VERSION -> stringResource(R.string.error_reject_version)
        else -> stringResource(R.string.error_reject_other, e.reason)
    }
    ErrorKind.SOCKET -> stringResource(R.string.error_socket, e.detail ?: "")
    ErrorKind.DISCONNECTED -> stringResource(R.string.error_disconnected)
    ErrorKind.BT_UNAVAILABLE -> stringResource(R.string.error_bt_unavailable)
    ErrorKind.BT_OFF -> stringResource(R.string.error_bt_off)
    ErrorKind.BT_PERMISSION -> stringResource(R.string.error_bt_permission)
    ErrorKind.BT_REGISTER -> stringResource(R.string.error_bt_register)
    ErrorKind.BT_CONNECT -> stringResource(R.string.error_bt_connect)
}
