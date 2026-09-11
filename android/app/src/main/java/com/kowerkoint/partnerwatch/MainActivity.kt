package com.kowerkoint.partnerwatch

import android.graphics.Color
import android.os.Bundle
import android.content.Intent
import android.provider.Settings
import android.net.Uri
import android.Manifest
import android.content.pm.PackageManager
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
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
import androidx.lifecycle.compose.LifecycleResumeEffect
import com.kowerkoint.partnerwatch.ui.EnrollmentScreen
import com.kowerkoint.partnerwatch.ui.EnrollmentViewModel

class MainActivity : ComponentActivity() {
    private val enrollmentViewModel: EnrollmentViewModel by viewModels()
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge(
            statusBarStyle = SystemBarStyle.light(Color.TRANSPARENT, Color.TRANSPARENT),
            navigationBarStyle = SystemBarStyle.light(Color.TRANSPARENT, Color.TRANSPARENT),
        )
        handleFilterIntent(intent)
        val notificationPermission = registerForActivityResult(
            ActivityResultContracts.RequestPermission(),
        ) { }
        val locationPermission=registerForActivityResult(ActivityResultContracts.RequestMultiplePermissions()){ }
        setContent {
            PartnerWatchTheme {
                val viewModel = enrollmentViewModel
                val state = viewModel.state.collectAsStateWithLifecycle()
                val registered = state.value as? EnrollmentUiState.Registered
                LifecycleResumeEffect(Unit) {
                    viewModel.refreshNotificationAccess()
                    onPauseOrDispose { }
                }
                LaunchedEffect(registered?.connectionMode) {
                    if (registered != null && ContextCompat.checkSelfPermission(
                            this@MainActivity,
                            Manifest.permission.POST_NOTIFICATIONS,
                        ) != PackageManager.PERMISSION_GRANTED
                    ) {
                        notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
                    }
                    if (registered?.connectionMode == ConnectionMode.ALWAYS_CONNECTED) {
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
                    onConnectionModeChanged = viewModel::setConnectionMode,
                    onBatterySharingChanged = viewModel::setBatterySharing,
                    onRequestPartnerStatus = viewModel::requestPartnerStatus,
                    onLocationSharingChanged={enabled->viewModel.setLocationSharing(enabled);if(enabled)locationPermission.launch(arrayOf(Manifest.permission.ACCESS_COARSE_LOCATION,Manifest.permission.ACCESS_FINE_LOCATION))},
                    onPreciseLocationChanged={precise->viewModel.setPreciseLocation(precise);if(precise)locationPermission.launch(arrayOf(Manifest.permission.ACCESS_COARSE_LOCATION,Manifest.permission.ACCESS_FINE_LOCATION))},
                    onOpenLocationSettings={startActivity(Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS,Uri.parse("package:$packageName")))},
                    onOpenMap={latitude,longitude->runCatching{startActivity(Intent(Intent.ACTION_VIEW,Uri.parse("geo:$latitude,$longitude?q=$latitude,$longitude")))}},
                    onNotificationForwardingChanged=viewModel::setNotificationForwarding,
                    onOpenNotificationAccessSettings={startActivity(Intent(Settings.ACTION_NOTIFICATION_LISTENER_SETTINGS))},
                    onAddNotificationFilter={viewModel.startNotificationFilter()},
                    onEditNotificationFilter=viewModel::startNotificationFilter,
                    onUpdateNotificationFilter=viewModel::updateNotificationFilter,
                    onSaveNotificationFilter=viewModel::saveNotificationFilter,
                    onCancelNotificationFilter=viewModel::cancelNotificationFilter,
                    onDeleteNotificationFilter=viewModel::deleteNotificationFilter,
                    onNotificationFilterEnabledChanged=viewModel::setNotificationFilterEnabled,
                    onUseSuggestedFilterTitle=viewModel::useSuggestedFilterTitle,
                )
            }
        }
    }

    override fun onNewIntent(intent: Intent) { super.onNewIntent(intent);setIntent(intent);handleFilterIntent(intent) }
    private fun handleFilterIntent(intent: Intent?) { if(intent?.action!=ACTION_ADD_NOTIFICATION_FILTER)return;enrollmentViewModel.startNotificationFilterFromNotification(intent.getStringExtra(EXTRA_SOURCE_DEVICE_ID).orEmpty(),intent.getStringExtra(EXTRA_SOURCE_DEVICE_NAME).orEmpty(),intent.getStringExtra(EXTRA_SOURCE_PACKAGE).orEmpty(),intent.getStringExtra(EXTRA_SOURCE_APP_NAME).orEmpty(),intent.getStringExtra(EXTRA_NOTIFICATION_TITLE).orEmpty());intent.action=null }

    companion object {
        const val ACTION_ADD_NOTIFICATION_FILTER="com.kowerkoint.partnerwatch.ADD_NOTIFICATION_FILTER"
        const val EXTRA_SOURCE_DEVICE_ID="sourceDeviceId";const val EXTRA_SOURCE_DEVICE_NAME="sourceDeviceName"
        const val EXTRA_SOURCE_PACKAGE="sourcePackage";const val EXTRA_SOURCE_APP_NAME="sourceAppName";const val EXTRA_NOTIFICATION_TITLE="notificationTitle"
    }
}

@Composable
private fun PartnerWatchTheme(content: @Composable () -> Unit) {
    MaterialTheme(content = content)
}
