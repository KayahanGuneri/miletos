package com.miletos.features.workflowruntime.client;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.JsonNodeFactory;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.google.protobuf.ListValue;
import com.google.protobuf.Struct;
import com.google.protobuf.Timestamp;
import com.google.protobuf.Value;
import com.google.protobuf.util.Timestamps;

public final class ProtobufValueConverter {

  private ProtobufValueConverter() {}

  static String timestamp(Timestamp value) {
    return value == null ? null : Timestamps.toString(value);
  }

  public static JsonNode struct(Struct value) {
    if (value == null) {
      return null;
    }
    ObjectNode result = JsonNodeFactory.instance.objectNode();
    value.getFieldsMap().forEach((key, child) -> result.set(key, value(child)));
    return result;
  }

  private static JsonNode list(ListValue value) {
    ArrayNode result = JsonNodeFactory.instance.arrayNode();
    value.getValuesList().forEach(child -> result.add(value(child)));
    return result;
  }

  private static JsonNode value(Value value) {
    return switch (value.getKindCase()) {
      case NULL_VALUE, KIND_NOT_SET -> JsonNodeFactory.instance.nullNode();
      case NUMBER_VALUE -> JsonNodeFactory.instance.numberNode(value.getNumberValue());
      case STRING_VALUE -> JsonNodeFactory.instance.textNode(value.getStringValue());
      case BOOL_VALUE -> JsonNodeFactory.instance.booleanNode(value.getBoolValue());
      case STRUCT_VALUE -> struct(value.getStructValue());
      case LIST_VALUE -> list(value.getListValue());
    };
  }
}
