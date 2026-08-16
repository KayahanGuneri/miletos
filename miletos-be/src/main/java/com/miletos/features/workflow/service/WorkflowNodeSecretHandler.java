package com.miletos.features.workflow.service;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.miletos.common.exception.MiletosException;
import com.miletos.features.workflow.exception.DatabasePasswordRequiredException;
import com.miletos.features.workflow.exception.SecretEncryptionUnavailableException;
import com.miletos.features.workflow.exception.SftpPasswordRequiredException;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.function.BiPredicate;
import java.util.function.Supplier;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;

@Component
@RequiredArgsConstructor
public class WorkflowNodeSecretHandler {

  private static final List<SecretDefinition> SECRET_DEFINITIONS =
      List.of(
          new SecretDefinition(
              "sftpPassword",
              "sftpPasswordEncrypted",
              WorkflowNodeSecretHandler::usesSftp,
              SftpPasswordRequiredException::new),
          new SecretDefinition(
              "databasePassword",
              "databasePasswordEncrypted",
              WorkflowNodeSecretHandler::usesDatabase,
              DatabasePasswordRequiredException::new));

  private final WorkflowSecretCipher workflowSecretCipher;

  public JsonNode encryptOnSave(JsonNode incomingDefinition, JsonNode previousDefinition) {
    if (incomingDefinition == null || !incomingDefinition.isObject()) {
      return incomingDefinition;
    }

    ObjectNode definition = incomingDefinition.deepCopy();
    JsonNode nodesNode = definition.get("nodes");
    if (nodesNode == null || !nodesNode.isArray()) {
      return definition;
    }

    Map<String, Map<String, String>> previousSecrets = previousEncryptedSecrets(previousDefinition);
    ArrayNode nodes = (ArrayNode) nodesNode;
    for (JsonNode node : nodes) {
      if (node == null || !node.isObject()) {
        continue;
      }
      ObjectNode nodeObject = (ObjectNode) node;
      JsonNode configurationNode = nodeObject.get("configuration");
      if (configurationNode == null || !configurationNode.isObject()) {
        continue;
      }
      ObjectNode configuration = (ObjectNode) configurationNode;
      String nodeId = nodeObject.path("nodeId").asText("");
      for (SecretDefinition secret : SECRET_DEFINITIONS) {
        protectSecret(nodeObject, configuration, nodeId, previousSecrets, secret);
      }
    }
    return definition;
  }

  private void protectSecret(
      ObjectNode node,
      ObjectNode configuration,
      String nodeId,
      Map<String, Map<String, String>> previousSecrets,
      SecretDefinition secret) {
    if (!secret.required().test(node, configuration)) {
      configuration.remove(secret.plaintextField());
      configuration.remove(secret.encryptedField());
      return;
    }

    String plaintext = textOrEmpty(configuration, secret.plaintextField());
    configuration.remove(secret.plaintextField());
    configuration.remove(secret.encryptedField());
    if (!plaintext.isBlank()) {
      if (!workflowSecretCipher.isConfigured()) {
        throw new SecretEncryptionUnavailableException();
      }
      configuration.put(secret.encryptedField(), workflowSecretCipher.encrypt(plaintext));
      return;
    }

    String preserved =
        previousSecrets.getOrDefault(nodeId, Map.of()).getOrDefault(secret.encryptedField(), "");
    if (!preserved.isBlank()) {
      configuration.put(secret.encryptedField(), preserved);
      return;
    }
    throw secret.requiredException().get();
  }

  private static Map<String, Map<String, String>> previousEncryptedSecrets(
      JsonNode previousDefinition) {
    Map<String, Map<String, String>> encrypted = new HashMap<>();
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
      if (nodeId.isBlank()) {
        continue;
      }
      Map<String, String> nodeSecrets = new HashMap<>();
      JsonNode configuration = node.path("configuration");
      for (SecretDefinition secret : SECRET_DEFINITIONS) {
        String value = textOrEmpty(configuration, secret.encryptedField());
        if (!value.isBlank()) {
          nodeSecrets.put(secret.encryptedField(), value);
        }
      }
      if (!nodeSecrets.isEmpty()) {
        encrypted.put(nodeId, Map.copyOf(nodeSecrets));
      }
    }
    return encrypted;
  }

  private static boolean usesSftp(ObjectNode node, ObjectNode configuration) {
    return "core.csv-output".equals(textOrEmpty(node, "pluginType"))
        || "SFTP".equalsIgnoreCase(textOrEmpty(configuration, "sourceType"))
        || "SFTP".equalsIgnoreCase(textOrEmpty(configuration, "destinationType"));
  }

  private static boolean usesDatabase(ObjectNode node, ObjectNode configuration) {
    String pluginType = textOrEmpty(node, "pluginType");
    return "core.database-input".equals(pluginType) || "core.database-output".equals(pluginType);
  }

  private static String textOrEmpty(JsonNode node, String field) {
    if (node == null || !node.isObject()) {
      return "";
    }
    JsonNode value = node.get(field);
    return value == null || value.isNull() ? "" : value.asText("").trim();
  }

  private record SecretDefinition(
      String plaintextField,
      String encryptedField,
      BiPredicate<ObjectNode, ObjectNode> required,
      Supplier<? extends MiletosException> requiredException) {}
}
