package com.kowerkoint.partnerwatch

import android.graphics.Color
import android.os.Bundle
import android.content.Intent
import android.provider.Settings
import android.net.Uri
import android.Manifest
import android.content.pm.PackageManager
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.LaunchedEffect
import androidx.core.content.ContextCompat
import com.kowerkoint.partnerwatch.connection.PartnerConnectionService
import com.kowerkoint.partnerwatch.connection.ConnectionMode
import com.kowerkoint.partnerwatch.ui.EnrollmentUiState
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.kowerkoint.partnerwatch.ui.EnrollmentScreen
import com.kowerkoint.partnerwatch.ui.EnrollmentViewModel

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge(
            statusBarStyle = SystemBarStyle.light(Color.TRANSPARENT, Color.TRANSPARENT),
            navigationBarStyle = SystemBarStyle.light(Color.TRANSPARENT, Color.TRANSPARENT),
        )
        val notificationPermission = registerForActivityResult(
            ActivityResultContracts.RequestPermission(),
        ) { }
        val locationPermission=registerForActivityResult(ActivityResultContracts.RequestMultiplePermissions()){ }
        setContent {
            PartnerWatchTheme {
                val viewModel: EnrollmentViewModel = viewModel()
                val state = viewModel.state.collectAsStateWithLifecycle()
                val registered = state.value as? EnrollmentUiState.Registered
                LaunchedEffect(registered?.connectionMode) {
                    if (registered?.connectionMode == ConnectionMode.ALWAYS_CONNECTED) {
                        if (ContextCompat.checkSelfPermission(
                                this@MainActivity,
                                Manifest.permission.POST_NOTIFICATIONS,
                            ) != PackageManager.PERMISSION_GRANTED
                        ) {
                            notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
                        }
                        ContextCompat.startForegroundService(
                            this@MainActivity,
                            Intent(this@MainActivity, PartnerConnectionService::class.java),
                        )
                    } else if (registered?.connectionMode == ConnectionMode.FCM_ONLY) {
                        stopService(Intent(this@MainActivity, PartnerConnectionService::class.java))
                    }
                }
                EnrollmentScreen(
                    state = state.value,
                    onServerUrlChanged = viewModel::updateServerUrl,
                    onInvitationCodeChanged = viewModel::updateInvitationCode,
                    onDeviceNameChanged = viewModel::updateDeviceName,
                    onEnroll = viewModel::enroll,
                    onCaptureAcceptingChanged = viewModel::setCaptureAccepting,
                    onOpenAccessibilitySettings = {
                        startActivity(Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS))
                    },
                    onRequestCapture = viewModel::requestCapture,
                    onSavePhoto = viewModel::saveReceivedPhoto,
                    onLogout = viewModel::logout,
                    onDisconnectForTest = viewModel::disconnectForTest,
                    onConnectionModeChanged = viewModel::setConnectionMode,
                    onBatterySharingChanged = viewModel::setBatterySharing,
                    onRequestPartnerStatus = viewModel::requestPartnerStatus,
                    onLocationSharingChanged={enabled->viewModel.setLocationSharing(enabled);if(enabled)locationPermission.launch(arrayOf(Manifest.permission.ACCESS_COARSE_LOCATION,Manifest.permission.ACCESS_FINE_LOCATION))},
                    onPreciseLocationChanged={precise->viewModel.setPreciseLocation(precise);if(precise)locationPermission.launch(arrayOf(Manifest.permission.ACCESS_COARSE_LOCATION,Manifest.permission.ACCESS_FINE_LOCATION))},
                    onOpenLocationSettings={startActivity(Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS,Uri.parse("package:$packageName")))},
                    onOpenMap={latitude,longitude->runCatching{startActivity(Intent(Intent.ACTION_VIEW,Uri.parse("geo:$latitude,$longitude?q=$latitude,$longitude")))}},
                )
            }
        }
    }
}

@Composable
private fun PartnerWatchTheme(content: @Composable () -> Unit) {
    MaterialTheme(content = content)
}
