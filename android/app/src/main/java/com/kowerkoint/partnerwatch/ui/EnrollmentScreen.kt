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
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import com.kowerkoint.partnerwatch.data.SavedEnrollment

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun EnrollmentScreen(
    state: EnrollmentUiState,
    onServerUrlChanged: (String) -> Unit,
    onInvitationCodeChanged: (String) -> Unit,
    onDeviceNameChanged: (String) -> Unit,
    onEnroll: () -> Unit,
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
                enrollment = state.enrollment,
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
private fun RegisteredContent(enrollment: SavedEnrollment, modifier: Modifier = Modifier) {
    Column(
        modifier = modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Text("登録完了", style = MaterialTheme.typography.headlineMedium)
        Text("この端末はPartner Watchに登録されています。")
        HorizontalDivider()
        DetailRow("サーバー", enrollment.serverUrl)
        DetailRow("端末ID", enrollment.deviceId)
        DetailRow("ペアID", enrollment.pairId)
        DetailRow("スロット", enrollment.slot.toString())
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
        )
    }
}
