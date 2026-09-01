package com.kowerkoint.partnerwatch

import android.graphics.Color
import android.os.Bundle
import android.content.Intent
import android.provider.Settings
import android.Manifest
import android.content.pm.PackageManager
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.LaunchedEffect
import androidx.core.content.ContextCompat
import com.kowerkoint.partnerwatch.connection.PartnerConnectionService
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
        setContent {
            PartnerWatchTheme {
                val viewModel: EnrollmentViewModel = viewModel()
                val state = viewModel.state.collectAsStateWithLifecycle()
                val isRegistered = state.value is EnrollmentUiState.Registered
                LaunchedEffect(isRegistered) {
                    if (isRegistered) {
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
                )
            }
        }
    }
}

@Composable
private fun PartnerWatchTheme(content: @Composable () -> Unit) {
    MaterialTheme(content = content)
}
