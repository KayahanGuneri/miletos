package com.miletos.features.workflow.service;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.miletos.features.workflow.exception.SecretEncryptionUnavailableException;
import com.miletos.features.workflow.exception.SftpPasswordRequiredException;
import java.util.HashMap;
import java.util.Map;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;

@Component
@RequiredArgsConstructor
public class WorkflowSftpSecretHandler {

  private final SftpPasswordCipher sftpPasswordCipher;

  public JsonNode encryptOnSave(JsonNode incomingDefinition, JsonNode previousDefinition) {
    if (incomingDefinition == null || !incomingDefinition.isObject()) {
      return incomingDefinition;
    }

    ObjectNode definition = incomingDefinition.deepCopy();
    JsonNode nodesNode = definition.get("nodes");
    if (nodesNode == null || !nodesNode.isArray()) {
      return definition;
    }

    Map<String, String> previousEncryptedByNodeId = previousEncryptedPasswords(previousDefinition);
    ArrayNode nodes = (ArrayNode) nodesNode;
    for (int index = 0; index < nodes.size(); index++) {
      JsonNode node = nodes.get(index);
      if (node == null || !node.isObject()) {
        continue;
      }
      ObjectNode nodeObject = (ObjectNode) node;
      JsonNode configurationNode = nodeObject.get("configuration");
      if (configurationNode == null || !configurationNode.isObject()) {
        continue;
      }

      ObjectNode configuration = (ObjectNode) configurationNode;
      if (!configuration.has("sourceType")
          && !configuration.has("sftpPassword")
          && !configuration.has("sftpPasswordEncrypted")) {
        continue;
      }
      String sourceType = textOrEmpty(configuration, "sourceType");
      if (!"SFTP".equalsIgnoreCase(sourceType)) {
        configuration.remove("sftpPassword");
        configuration.remove("sftpPasswordEncrypted");
        continue;
      }

      String nodeId = nodeObject.path("nodeId").asText("");
      String plaintext = textOrEmpty(configuration, "sftpPassword");
      if (!plaintext.isBlank()) {
        if (!sftpPasswordCipher.isConfigured()) {
          throw new SecretEncryptionUnavailableException();
        }
        configuration.put("sftpPasswordEncrypted", sftpPasswordCipher.encrypt(plaintext));
        configuration.remove("sftpPassword");
        continue;
      }

      configuration.remove("sftpPassword");
      configuration.remove("sftpPasswordEncrypted");
      String preserved = previousEncryptedByNodeId.get(nodeId);
      if (preserved != null && !preserved.isBlank()) {
        configuration.put("sftpPasswordEncrypted", preserved);
      } else {
        throw new SftpPasswordRequiredException();
      }
    }

    return definition;
  }

  private static Map<String, String> previousEncryptedPasswords(JsonNode previousDefinition) {
    Map<String, String> encrypted = new HashMap<>();
    if (previousDefinition == null || !previousDefinition.isObject()) {
      return encrypted;
    }
    JsonNode nodes = previousDefinition.get("nodes");
    if (nodes == null || !nodes.isArray()) {
      return encrypted;
    }
    for (JsonNode node : nodes) {
      if (node == null || !node.isObject()) {
        continue;
      }
      String nodeId = node.path("nodeId").asText("");
      String value = textOrEmpty(node.path("configuration"), "sftpPasswordEncrypted");
      if (!nodeId.isBlank() && !value.isBlank()) {
        encrypted.put(nodeId, value);
      }
    }
    return encrypted;
  }

  private static String textOrEmpty(JsonNode node, String field) {
    if (node == null || !node.isObject()) {
      return "";
    }
    JsonNode value = node.get(field);
    if (value == null || value.isNull()) {
      return "";
    }
    return value.asText("").trim();
  }
}
