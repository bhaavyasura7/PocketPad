package io.pocketpad.app

import android.app.Activity
import android.content.pm.ActivityInfo
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.viewmodel.compose.viewModel
import io.pocketpad.app.ui.AppViewModel
import io.pocketpad.app.ui.ConnectScreen
import io.pocketpad.app.ui.ControllerScreen
import io.pocketpad.app.ui.ControlsScreen
import io.pocketpad.app.ui.PocketPadTheme
import io.pocketpad.app.ui.Screen
import io.pocketpad.app.ui.SettingsScreen

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            PocketPadTheme {
                val vm: AppViewModel = viewModel()
                AppRoot(vm)
            }
        }
    }
}

@Composable
private fun AppRoot(vm: AppViewModel) {
    val context = LocalContext.current

    // Landscape only while controlling; the connect and settings screens
    // follow the device orientation.
    LaunchedEffect(vm.screen) {
        (context as? Activity)?.requestedOrientation = when (vm.screen) {
            Screen.Controller -> ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE
            else -> ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED
        }
    }

    BackHandler(enabled = vm.screen != Screen.Connect) {
        when (vm.screen) {
            Screen.Controller -> vm.exitController()
            Screen.Settings -> vm.closeSettings()
            Screen.Controls -> vm.closeControls()
            else -> Unit
        }
    }

    Surface(modifier = Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) {
        when (vm.screen) {
            Screen.Connect -> ConnectScreen(vm)
            Screen.Controller -> ControllerScreen(vm)
            Screen.Settings -> SettingsScreen(vm)
            Screen.Controls -> ControlsScreen(vm)
        }
    }
}
