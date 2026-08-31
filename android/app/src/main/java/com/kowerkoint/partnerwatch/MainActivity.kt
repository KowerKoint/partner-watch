package com.kowerkoint.partnerwatch

import android.graphics.Color
import android.os.Bundle
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
        setContent {
            PartnerWatchTheme {
                val viewModel: EnrollmentViewModel = viewModel()
                val state = viewModel.state.collectAsStateWithLifecycle()
                EnrollmentScreen(
                    state = state.value,
                    onServerUrlChanged = viewModel::updateServerUrl,
                    onInvitationCodeChanged = viewModel::updateInvitationCode,
                    onDeviceNameChanged = viewModel::updateDeviceName,
                    onEnroll = viewModel::enroll,
                )
            }
        }
    }
}

@Composable
private fun PartnerWatchTheme(content: @Composable () -> Unit) {
    MaterialTheme(content = content)
}
