package com.miletos.features.workflowruntime.client;

import com.miletos.features.workflowruntime.grpc.generated.CreateHTTPTriggerResponse;
import com.miletos.features.workflowruntime.grpc.generated.CreateCronTriggerResponse;
import com.miletos.features.workflowruntime.grpc.generated.CronTriggerResponse;
import com.miletos.features.workflowruntime.grpc.generated.EdgeConstraint;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionDefinition;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionError;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionErrorPage;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionEvent;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionEventPage;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionLog;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionLogPage;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionPage;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionResponse;
import com.miletos.features.workflowruntime.grpc.generated.ExecutionSummary;
import com.miletos.features.workflowruntime.grpc.generated.HTTPTriggerResponse;
import com.miletos.features.workflowruntime.grpc.generated.ListPluginsResponse;
import com.miletos.features.workflowruntime.grpc.generated.NodeExecution;
import com.miletos.features.workflowruntime.grpc.generated.NodeExecutionPage;
import com.miletos.features.workflowruntime.grpc.generated.NodePosition;
import com.miletos.features.workflowruntime.grpc.generated.Plugin;
import com.miletos.features.workflowruntime.grpc.generated.Port;
import com.miletos.features.workflowruntime.grpc.generated.RecoveryResponse;
import com.miletos.features.workflowruntime.grpc.generated.WorkflowDefinition;
import com.miletos.features.workflowruntime.grpc.generated.WorkflowEdge;
import com.miletos.features.workflowruntime.grpc.generated.WorkflowNode;
import org.mapstruct.Mapper;
import org.mapstruct.Mapping;
import org.mapstruct.ReportingPolicy;

@Mapper(
    componentModel = "spring",
    uses = ProtobufValueConverter.class,
    unmappedTargetPolicy = ReportingPolicy.ERROR)
public interface WorkflowRuntimeGrpcJsonMapper {

  // Plugin mappings

  @Mapping(target = "items", source = "itemsList")
  WorkflowRuntimeBrowserDtos.PluginPage map(ListPluginsResponse source);

  @Mapping(target = "inputPorts", source = "inputPortsList")
  @Mapping(target = "outputPorts", source = "outputPortsList")
  @Mapping(target = "allowedRootOrigins", source = "allowedRootOriginsList")
  WorkflowRuntimeBrowserDtos.Plugin map(Plugin source);

  WorkflowRuntimeBrowserDtos.Port map(Port source);

  WorkflowRuntimeBrowserDtos.EdgeConstraint map(EdgeConstraint source);

  // Execution mappings

  WorkflowRuntimeBrowserDtos.ExecutionResponse map(ExecutionResponse source);

  WorkflowRuntimeBrowserDtos.ExecutionSummary map(ExecutionSummary source);

  @Mapping(target = "items", source = "itemsList")
  @Mapping(target = "next", source = "cursor")
  WorkflowRuntimeBrowserDtos.ExecutionPage map(ExecutionPage source);

  WorkflowRuntimeBrowserDtos.ExecutionDefinition map(ExecutionDefinition source);

  WorkflowRuntimeBrowserDtos.RecoveryResponse map(RecoveryResponse source);

  // Workflow-definition mappings

  @Mapping(target = "id", source = "workflowId")
  @Mapping(target = "revision", source = "workflowRevision")
  @Mapping(target = "nodes", source = "nodesList")
  @Mapping(target = "edges", source = "edgesList")
  WorkflowRuntimeBrowserDtos.WorkflowDefinition map(WorkflowDefinition source);

  @Mapping(target = "id", source = "nodeId")
  WorkflowRuntimeBrowserDtos.WorkflowNode map(WorkflowNode source);

  @Mapping(target = "id", source = "edgeId")
  WorkflowRuntimeBrowserDtos.WorkflowEdge map(WorkflowEdge source);

  WorkflowRuntimeBrowserDtos.NodePosition map(NodePosition source);

  // Node, event, log, and error mappings

  @Mapping(target = "items", source = "itemsList")
  @Mapping(target = "next", source = "cursor")
  WorkflowRuntimeBrowserDtos.NodeExecutionPage map(NodeExecutionPage source);

  @Mapping(target = "workflowExecutionId", source = "executionId")
  WorkflowRuntimeBrowserDtos.NodeExecution map(NodeExecution source);

  @Mapping(target = "items", source = "itemsList")
  @Mapping(target = "next", source = "cursor")
  WorkflowRuntimeBrowserDtos.ExecutionEventPage map(ExecutionEventPage source);

  @Mapping(target = "workflowExecutionId", source = "executionId")
  WorkflowRuntimeBrowserDtos.ExecutionEvent map(ExecutionEvent source);

  @Mapping(target = "items", source = "itemsList")
  @Mapping(target = "next", source = "cursor")
  WorkflowRuntimeBrowserDtos.ExecutionLogPage map(ExecutionLogPage source);

  @Mapping(target = "workflowExecutionId", source = "executionId")
  WorkflowRuntimeBrowserDtos.ExecutionLog map(ExecutionLog source);

  @Mapping(target = "items", source = "itemsList")
  @Mapping(target = "next", source = "cursor")
  WorkflowRuntimeBrowserDtos.ExecutionErrorPage map(ExecutionErrorPage source);

  @Mapping(target = "workflowExecutionId", source = "executionId")
  WorkflowRuntimeBrowserDtos.ExecutionError map(ExecutionError source);

  // HTTP-trigger mappings

  WorkflowRuntimeBrowserDtos.HTTPTriggerResponse map(HTTPTriggerResponse source);

  WorkflowRuntimeBrowserDtos.CreateHTTPTriggerResponse map(CreateHTTPTriggerResponse source);

  WorkflowRuntimeBrowserDtos.CronTriggerResponse map(CronTriggerResponse source);

  WorkflowRuntimeBrowserDtos.CreateCronTriggerResponse map(CreateCronTriggerResponse source);
}
