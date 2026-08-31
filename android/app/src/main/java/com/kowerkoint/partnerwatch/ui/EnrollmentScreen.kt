package com.kowerkoint.partnerwatch.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun EnrollmentScreen(
    state: EnrollmentUiState,
    onServerUrlChanged: (String) -> Unit,
    onInvitationCodeChanged: (String) -> Unit,
    onDeviceNameChanged: (String) -> Unit,
    onEnroll: () -> Unit,
    onCaptureAcceptingChanged: (Boolean) -> Unit,
    onOpenAccessibilitySettings: () -> Unit,
) {
    Scaffold(
        topBar = { TopAppBar(title = { Text("Partner Watch") }) },
    ) { padding ->
        when (state) {
            EnrollmentUiState.Loading -> LoadingContent(Modifier.padding(padding))
            is EnrollmentUiState.Form -> EnrollmentFormContent(
                form = state.value,
                onServerUrlChanged = onServerUrlChanged,
                onInvitationCodeChanged = onInvitationCodeChanged,
                onDeviceNameChanged = onDeviceNameChanged,
                onEnroll = onEnroll,
                modifier = Modifier.padding(padding),
            )
            is EnrollmentUiState.Registered -> RegisteredContent(
                state = state,
                onCaptureAcceptingChanged = onCaptureAcceptingChanged,
                onOpenAccessibilitySettings = onOpenAccessibilitySettings,
                modifier = Modifier.padding(padding),
            )
        }
    }
}

@Composable
private fun LoadingContent(modifier: Modifier = Modifier) {
    Box(modifier = modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        CircularProgressIndicator()
    }
}

@Composable
private fun EnrollmentFormContent(
    form: EnrollmentForm,
    onServerUrlChanged: (String) -> Unit,
    onInvitationCodeChanged: (String) -> Unit,
    onDeviceNameChanged: (String) -> Unit,
    onEnroll: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Text("端末を登録", style = MaterialTheme.typography.headlineMedium)
        Text(
            "管理者CLIで発行した招待コードを使って、この端末をペアへ登録します。",
            style = MaterialTheme.typography.bodyMedium,
        )
        OutlinedTextField(
            value = form.serverUrl,
            onValueChange = onServerUrlChanged,
            modifier = Modifier.fillMaxWidth(),
            label = { Text("サーバーURL") },
            placeholder = { Text("https://example.com") },
            supportingText = { Text("HTTPSのホスト名を入力してください") },
            singleLine = true,
            enabled = !form.isSubmitting,
        )
        OutlinedTextField(
            value = form.invitationCode,
            onValueChange = onInvitationCodeChanged,
            modifier = Modifier.fillMaxWidth(),
            label = { Text("招待コード") },
            visualTransformation = PasswordVisualTransformation(),
            singleLine = true,
            enabled = !form.isSubmitting,
        )
        OutlinedTextField(
            value = form.deviceName,
            onValueChange = onDeviceNameChanged,
            modifier = Modifier.fillMaxWidth(),
            label = { Text("端末名") },
            supportingText = { Text("相手側に表示される名前です") },
            singleLine = true,
            enabled = !form.isSubmitting,
        )
        form.errorMessage?.let {
            Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium)
        }
        Button(
            onClick = onEnroll,
            modifier = Modifier.fillMaxWidth(),
            enabled = !form.isSubmitting,
        ) {
            if (form.isSubmitting) {
                CircularProgressIndicator(
                    modifier = Modifier.height(20.dp),
                    strokeWidth = 2.dp,
                )
                Spacer(Modifier.padding(horizontal = 6.dp))
            }
            Text(if (form.isSubmitting) "登録しています…" else "この端末を登録")
        }
    }
}

@Composable
private fun RegisteredContent(
    state: EnrollmentUiState.Registered,
    onCaptureAcceptingChanged: (Boolean) -> Unit,
    onOpenAccessibilitySettings: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(
        modifier = modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Text("端末の設定", style = MaterialTheme.typography.headlineMedium)
        Text("この端末はPartner Watchに登録されています。")
        HorizontalDivider()
        Text("画面撮影", style = MaterialTheme.typography.titleLarge)
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(modifier = Modifier.weight(1f)) {
                Text("撮影要求を受け付ける", style = MaterialTheme.typography.titleMedium)
                Text(
                    "本人が有効にしている間だけ撮影します",
                    style = MaterialTheme.typography.bodySmall,
                )
            }
            Switch(
                checked = state.acceptingCaptures,
                onCheckedChange = onCaptureAcceptingChanged,
            )
        }
        Text(
            if (state.accessibilityConnected) "撮影サービス: 有効" else "撮影サービス: 無効",
            color = if (state.accessibilityConnected) {
                MaterialTheme.colorScheme.primary
            } else {
                MaterialTheme.colorScheme.error
            },
        )
        if (!state.accessibilityConnected) {
            Text("Androidのユーザー補助設定で撮影サービスを許可してください。")
            Button(onClick = onOpenAccessibilitySettings, modifier = Modifier.fillMaxWidth()) {
                Text("ユーザー補助設定を開く")
            }
        }
        HorizontalDivider()
        Text("登録情報", style = MaterialTheme.typography.titleLarge)
        DetailRow("サーバー", state.enrollment.serverUrl)
        DetailRow("端末ID", state.enrollment.deviceId)
        DetailRow("ペアID", state.enrollment.pairId)
        DetailRow("スロット", state.enrollment.slot.toString())
    }
}

@Composable
private fun DetailRow(label: String, value: String) {
    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(label, modifier = Modifier.weight(0.3f), style = MaterialTheme.typography.labelLarge)
        Text(value, modifier = Modifier.weight(0.7f), style = MaterialTheme.typography.bodyMedium)
    }
}

@Preview(showBackground = true)
@Composable
private fun EnrollmentFormPreview() {
    MaterialTheme {
        EnrollmentScreen(
            state = EnrollmentUiState.Form(EnrollmentForm(deviceName = "Pixel 8a")),
            onServerUrlChanged = {},
            onInvitationCodeChanged = {},
            onDeviceNameChanged = {},
            onEnroll = {},
            onCaptureAcceptingChanged = {},
            onOpenAccessibilitySettings = {},
        )
    }
}
