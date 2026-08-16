package com.miletos.features.workflow.service;

import com.miletos.config.SecretsProperties;
import com.miletos.features.workflow.exception.SecretEncryptionUnavailableException;
import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.security.GeneralSecurityException;
import java.security.SecureRandom;
import java.util.Base64;
import javax.crypto.Cipher;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;
import javax.crypto.spec.SecretKeySpec;
import org.springframework.stereotype.Component;
import org.springframework.util.StringUtils;

@Component
public class WorkflowSecretCipher {

  private static final String TRANSFORMATION = "AES/GCM/NoPadding";
  private static final int GCM_IV_LENGTH_BYTES = 12;
  private static final int GCM_TAG_LENGTH_BITS = 128;
  private static final int AES_KEY_LENGTH_BYTES = 32;

  private final byte[] keyBytes;
  private final SecureRandom secureRandom = new SecureRandom();

  public WorkflowSecretCipher(SecretsProperties secretsProperties) {
    this.keyBytes = decodeKey(secretsProperties == null ? null : secretsProperties.aesKey());
  }

  public boolean isConfigured() {
    return keyBytes != null;
  }

  public String encrypt(String plaintext) {
    if (!StringUtils.hasText(plaintext)) {
      throw new IllegalArgumentException("plaintext must not be blank");
    }
    if (keyBytes == null) {
      throw new SecretEncryptionUnavailableException();
    }
    SecretKey key = new SecretKeySpec(keyBytes, "AES");
    try {
      byte[] iv = new byte[GCM_IV_LENGTH_BYTES];
      secureRandom.nextBytes(iv);
      Cipher cipher = Cipher.getInstance(TRANSFORMATION);
      cipher.init(Cipher.ENCRYPT_MODE, key, new GCMParameterSpec(GCM_TAG_LENGTH_BITS, iv));
      byte[] ciphertext = cipher.doFinal(plaintext.getBytes(StandardCharsets.UTF_8));
      ByteBuffer buffer = ByteBuffer.allocate(iv.length + ciphertext.length);
      buffer.put(iv);
      buffer.put(ciphertext);
      return Base64.getEncoder().encodeToString(buffer.array());
    } catch (GeneralSecurityException exception) {
      throw new IllegalStateException("AES-GCM encryption failed", exception);
    }
  }

  private static byte[] decodeKey(String configured) {
    if (!StringUtils.hasText(configured)) {
      return null;
    }
    try {
      byte[] decoded = Base64.getDecoder().decode(configured.trim());
      if (decoded.length != AES_KEY_LENGTH_BYTES) {
        throw new IllegalStateException("miletos.secrets.aes-key must decode to exactly 32 bytes");
      }
      return decoded;
    } catch (IllegalArgumentException exception) {
      throw new IllegalStateException("miletos.secrets.aes-key must be valid base64", exception);
    }
  }
}
