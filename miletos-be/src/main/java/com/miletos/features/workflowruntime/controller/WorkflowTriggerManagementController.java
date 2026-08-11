package com.miletos.features.workflowruntime.controller;

import com.fasterxml.jackson.databind.JsonNode;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.workflowruntime.client.WorkflowRuntimeResponse;
import com.miletos.features.workflowruntime.controller.request.CreateWorkflowTriggerRequest;
import com.miletos.features.workflowruntime.service.WorkflowTriggerManagementService;
import com.miletos.security.CurrentUser;
import com.miletos.security.authorization.Authorize;
import com.miletos.security.authorization.RequiredRole;
import jakarta.validation.Valid;
import lombok.RequiredArgsConstructor;
import org.springframework.http.HttpHeaders;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestHeader;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping(value = "/api/workflows/{workflowId}", produces = MediaType.APPLICATION_JSON_VALUE)
@RequiredArgsConstructor
public class WorkflowTriggerManagementController {

  private final WorkflowTriggerManagementService triggerManagementService;

  @Authorize(RequiredRole.COMPANY_ADMIN)
  @PostMapping(value = "/triggers/http", consumes = MediaType.APPLICATION_JSON_VALUE)
  public ResponseEntity<byte[]> createHTTPTrigger(
      @PathVariable Long workflowId,
      @Valid @RequestBody CreateWorkflowTriggerRequest request,
      @CurrentUser User user,
      @RequestHeader HttpHeaders browserHeaders) {
    return response(
        triggerManagementService.createHTTPTrigger(
            user, workflowId, request.triggerNodeId(), browserHeaders));
  }

  @Authorize(RequiredRole.COMPANY_ADMIN)
  @GetMapping("/triggers/http")
  public ResponseEntity<byte[]> getActiveHTTPTrigger(
      @PathVariable Long workflowId,
      @CurrentUser User user,
      @RequestHeader HttpHeaders browserHeaders) {
    return response(triggerManagementService.getActiveHTTPTrigger(user, workflowId, browserHeaders));
  }

  @Authorize(RequiredRole.COMPANY_ADMIN)
  @GetMapping("/triggers/http/{triggerId}")
  public ResponseEntity<byte[]> getHTTPTrigger(
      @PathVariable Long workflowId,
      @PathVariable String triggerId,
      @CurrentUser User user,
      @RequestHeader HttpHeaders browserHeaders) {
    return response(
        triggerManagementService.getHTTPTrigger(user, workflowId, triggerId, browserHeaders));
  }

  @Authorize(RequiredRole.COMPANY_ADMIN)
  @PostMapping("/triggers/http/{triggerId}/disable")
  public ResponseEntity<byte[]> disableHTTPTrigger(
      @PathVariable Long workflowId,
      @PathVariable String triggerId,
      @CurrentUser User user,
      @RequestHeader HttpHeaders browserHeaders) {
    return response(
        triggerManagementService.disableHTTPTrigger(user, workflowId, triggerId, browserHeaders));
  }

  @Authorize(RequiredRole.COMPANY_ADMIN)
  @PostMapping(value = "/triggers/cron", consumes = MediaType.APPLICATION_JSON_VALUE)
  public ResponseEntity<byte[]> createCronTrigger(
      @PathVariable Long workflowId,
      @Valid @RequestBody CreateWorkflowTriggerRequest request,
      @CurrentUser User user,
      @RequestHeader HttpHeaders browserHeaders) {
    return response(
        triggerManagementService.createCronTrigger(
            user, workflowId, request.triggerNodeId(), browserHeaders));
  }

  @Authorize(RequiredRole.COMPANY_ADMIN)
  @GetMapping("/triggers/cron")
  public ResponseEntity<byte[]> getActiveCronTrigger(
      @PathVariable Long workflowId,
      @CurrentUser User user,
      @RequestHeader HttpHeaders browserHeaders) {
    return response(triggerManagementService.getActiveCronTrigger(user, workflowId, browserHeaders));
  }

  @Authorize(RequiredRole.COMPANY_ADMIN)
  @GetMapping("/triggers/cron/{triggerId}")
  public ResponseEntity<byte[]> getCronTrigger(
      @PathVariable Long workflowId,
      @PathVariable String triggerId,
      @CurrentUser User user,
      @RequestHeader HttpHeaders browserHeaders) {
    return response(
        triggerManagementService.getCronTrigger(user, workflowId, triggerId, browserHeaders));
  }

  @Authorize(RequiredRole.COMPANY_ADMIN)
  @PostMapping("/triggers/cron/{triggerId}/disable")
  public ResponseEntity<byte[]> disableCronTrigger(
      @PathVariable Long workflowId,
      @PathVariable String triggerId,
      @CurrentUser User user,
      @RequestHeader HttpHeaders browserHeaders) {
    return response(
        triggerManagementService.disableCronTrigger(user, workflowId, triggerId, browserHeaders));
  }

  @Authorize(RequiredRole.COMPANY_ADMIN)
  @PostMapping(value = "/run", consumes = MediaType.APPLICATION_JSON_VALUE)
  public ResponseEntity<byte[]> runWorkflow(
      @PathVariable Long workflowId,
      @RequestBody(required = false) JsonNode request,
      @CurrentUser User user,
      @RequestHeader HttpHeaders browserHeaders) {
    return response(triggerManagementService.runWorkflow(user, workflowId, request, browserHeaders));
  }

  private ResponseEntity<byte[]> response(WorkflowRuntimeResponse response) {
    return ResponseEntity.status(response.status())
        .headers(response.headers())
        .body(response.body());
  }
}
