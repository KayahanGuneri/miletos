package com.miletos.features.workflow.controller;

import com.miletos.features.user.repository.entity.User;
import com.miletos.features.workflow.WorkflowMapper;
import com.miletos.features.workflow.WorkflowService;
import com.miletos.features.workflow.controller.request.CreateWorkflowRequest;
import com.miletos.features.workflow.controller.request.UpdateWorkflowRequest;
import com.miletos.features.workflow.controller.response.WorkflowInputFileResponse;
import com.miletos.features.workflow.controller.response.WorkflowPageResponse;
import com.miletos.features.workflow.controller.response.WorkflowResponse;
import com.miletos.features.workflow.repository.entity.Workflow;
import com.miletos.features.workflow.repository.entity.WorkflowStatus;
import com.miletos.features.workflow.service.WorkflowInputFileStorageService;
import com.miletos.security.AuthenticatedActorResolver;
import com.miletos.security.authorization.Authorize;
import com.miletos.security.authorization.RequiredRole;
import jakarta.validation.Valid;
import lombok.RequiredArgsConstructor;
import org.springframework.data.domain.Page;
import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.security.core.annotation.AuthenticationPrincipal;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestHeader;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.multipart.MultipartFile;

@RestController
@RequestMapping("/api/workflows")
@RequiredArgsConstructor
public class WorkflowController {

  private final WorkflowService workflowService;
  private final WorkflowMapper workflowMapper;
  private final AuthenticatedActorResolver authenticatedActorResolver;
  private final WorkflowInputFileStorageService workflowInputFileStorageService;

  @Authorize(RequiredRole.COMPANY_ADMIN)
  @PostMapping(path = "/input-files", consumes = MediaType.MULTIPART_FORM_DATA_VALUE)
  public ResponseEntity<WorkflowInputFileResponse> uploadInputFile(
      @RequestParam("file") MultipartFile file,
      @AuthenticationPrincipal(expression = "subject") String actorEmail) {

    User user = authenticatedActorResolver.resolve(actorEmail);
    String fileName = workflowInputFileStorageService.store(user, file);
    return ResponseEntity.status(HttpStatus.CREATED).body(new WorkflowInputFileResponse(fileName));
  }

  @Authorize(RequiredRole.COMPANY_ADMIN)
  @PostMapping
  public ResponseEntity<WorkflowResponse> createWorkflow(
      @Valid @RequestBody CreateWorkflowRequest request,
      @AuthenticationPrincipal(expression = "subject") String actorEmail,
      @RequestHeader HttpHeaders browserHeaders) {

    User user = authenticatedActorResolver.resolve(actorEmail);
    Workflow workflow = workflowMapper.toEntity(request, user);

    Workflow createdWorkflow = workflowService.createWorkflow(workflow, browserHeaders);

    return ResponseEntity.status(HttpStatus.CREATED)
        .body(workflowMapper.toResponse(createdWorkflow));
  }

  @Authorize(RequiredRole.COMPANY_ADMIN)
  @GetMapping
  public ResponseEntity<WorkflowPageResponse> listWorkflows(
      @RequestParam(defaultValue = "0") Integer page,
      @RequestParam(defaultValue = "20") Integer size,
      @RequestParam(required = false) String search,
      @RequestParam(required = false) WorkflowStatus status,
      @AuthenticationPrincipal(expression = "subject") String actorEmail) {

    User user = authenticatedActorResolver.resolve(actorEmail);

    Page<Workflow> workflows = workflowService.listWorkflows(user, page, size, search, status);

    return ResponseEntity.ok(workflowMapper.toPageResponse(workflows));
  }

  @Authorize(RequiredRole.COMPANY_ADMIN)
  @GetMapping("/{workflowId}")
  public ResponseEntity<WorkflowResponse> getWorkflow(
      @PathVariable Long workflowId,
      @AuthenticationPrincipal(expression = "subject") String actorEmail) {

    User user = authenticatedActorResolver.resolve(actorEmail);

    Workflow workflow = workflowService.getWorkflowById(user, workflowId);

    return ResponseEntity.ok(workflowMapper.toResponse(workflow));
  }

  @Authorize(RequiredRole.COMPANY_ADMIN)
  @PutMapping("/{workflowId}")
  public ResponseEntity<WorkflowResponse> updateWorkflow(
      @PathVariable Long workflowId,
      @Valid @RequestBody UpdateWorkflowRequest request,
      @AuthenticationPrincipal(expression = "subject") String actorEmail,
      @RequestHeader HttpHeaders browserHeaders) {

    User user = authenticatedActorResolver.resolve(actorEmail);
    Workflow changes = workflowMapper.toEntity(request, user);

    Workflow updatedWorkflow = workflowService.updateWorkflow(workflowId, changes, browserHeaders);

    return ResponseEntity.ok(workflowMapper.toResponse(updatedWorkflow));
  }

  @Authorize(RequiredRole.COMPANY_ADMIN)
  @DeleteMapping("/{workflowId}")
  public ResponseEntity<Void> deleteWorkflow(
      @PathVariable Long workflowId,
      @AuthenticationPrincipal(expression = "subject") String actorEmail,
      @RequestHeader HttpHeaders browserHeaders) {

    User user = authenticatedActorResolver.resolve(actorEmail);

    workflowService.deleteWorkflow(user, workflowId, browserHeaders);

    return ResponseEntity.noContent().build();
  }

  @Authorize(RequiredRole.COMPANY_ADMIN)
  @PostMapping("/{workflowId}/activate")
  public ResponseEntity<WorkflowResponse> activateWorkflow(
      @PathVariable Long workflowId,
      @AuthenticationPrincipal(expression = "subject") String actorEmail,
      @RequestHeader HttpHeaders browserHeaders) {

    User user = authenticatedActorResolver.resolve(actorEmail);

    Workflow workflow = workflowService.activateWorkflow(user, workflowId, browserHeaders);

    return ResponseEntity.ok(workflowMapper.toResponse(workflow));
  }

  @Authorize(RequiredRole.COMPANY_ADMIN)
  @PostMapping("/{workflowId}/archive")
  public ResponseEntity<WorkflowResponse> archiveWorkflow(
      @PathVariable Long workflowId,
      @AuthenticationPrincipal(expression = "subject") String actorEmail,
      @RequestHeader HttpHeaders browserHeaders) {

    User user = authenticatedActorResolver.resolve(actorEmail);

    Workflow workflow = workflowService.archiveWorkflow(user, workflowId, browserHeaders);

    return ResponseEntity.ok(workflowMapper.toResponse(workflow));
  }

  @Authorize(RequiredRole.COMPANY_ADMIN)
  @PostMapping("/{workflowId}/restore")
  public ResponseEntity<WorkflowResponse> restoreWorkflow(
      @PathVariable Long workflowId,
      @AuthenticationPrincipal(expression = "subject") String actorEmail) {

    User user = authenticatedActorResolver.resolve(actorEmail);

    Workflow workflow = workflowService.restoreWorkflow(user, workflowId);

    return ResponseEntity.ok(workflowMapper.toResponse(workflow));
  }
}
