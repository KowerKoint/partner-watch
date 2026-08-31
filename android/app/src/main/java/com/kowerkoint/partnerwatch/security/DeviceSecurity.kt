package com.kowerkoint.partnerwatch.security

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.spec.ECGenParameterSpec
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey

class DeviceSecurity {
    fun prepare(): String {
        val publicKey = publicKey()
        credentialKey()
        return publicKey
    }

    fun publicKey(): String {
        val keyStore = keyStore()
        if (!keyStore.containsAlias(SIGNING_KEY_ALIAS)) {
            val generator = KeyPairGenerator.getInstance(
                KeyProperties.KEY_ALGORITHM_EC,
                ANDROID_KEY_STORE,
            )
            generator.initialize(
                KeyGenParameterSpec.Builder(
                    SIGNING_KEY_ALIAS,
                    KeyProperties.PURPOSE_SIGN or KeyProperties.PURPOSE_VERIFY,
                )
                    .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                    .setDigests(KeyProperties.DIGEST_SHA256)
                    .setUserAuthenticationRequired(false)
                    .build(),
            )
            generator.generateKeyPair()
        }
        val certificate = keyStore.getCertificate(SIGNING_KEY_ALIAS)
            ?: error("Device signing certificate is unavailable")
        return Base64.encodeToString(certificate.publicKey.encoded, BASE64_FLAGS)
    }

    fun encryptCredential(credential: String): String {
        val cipher = Cipher.getInstance(AES_TRANSFORMATION)
        cipher.init(Cipher.ENCRYPT_MODE, credentialKey())
        val encrypted = cipher.doFinal(credential.toByteArray(Charsets.UTF_8))
        return listOf(cipher.iv, encrypted).joinToString(SEPARATOR) {
            Base64.encodeToString(it, BASE64_FLAGS)
        }
    }

    fun decryptCredential(value: String): String {
        val parts = value.split(SEPARATOR, limit = 2)
        require(parts.size == 2)
        val iv = Base64.decode(parts[0], BASE64_FLAGS)
        val encrypted = Base64.decode(parts[1], BASE64_FLAGS)
        val cipher = Cipher.getInstance(AES_TRANSFORMATION)
        cipher.init(
            Cipher.DECRYPT_MODE,
            credentialKey(),
            javax.crypto.spec.GCMParameterSpec(128, iv),
        )
        return cipher.doFinal(encrypted).toString(Charsets.UTF_8)
    }

    private fun credentialKey(): SecretKey {
        val keyStore = keyStore()
        (keyStore.getKey(CREDENTIAL_KEY_ALIAS, null) as? SecretKey)?.let { return it }
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, ANDROID_KEY_STORE)
        generator.init(
            KeyGenParameterSpec.Builder(
                CREDENTIAL_KEY_ALIAS,
                KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
            )
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setRandomizedEncryptionRequired(true)
                .setKeySize(256)
                .build(),
        )
        return generator.generateKey()
    }

    private fun keyStore(): KeyStore = KeyStore.getInstance(ANDROID_KEY_STORE).apply { load(null) }

    private companion object {
        const val ANDROID_KEY_STORE = "AndroidKeyStore"
        const val SIGNING_KEY_ALIAS = "partner_watch_device_signing_v1"
        const val CREDENTIAL_KEY_ALIAS = "partner_watch_credential_encryption_v1"
        const val AES_TRANSFORMATION = "AES/GCM/NoPadding"
        const val SEPARATOR = "."
        const val BASE64_FLAGS = Base64.URL_SAFE or Base64.NO_WRAP or Base64.NO_PADDING
    }
}
