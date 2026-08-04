package com.miletos.features.workflow.controller;

import org.springframework.data.domain.Page;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import com.miletos.features.workflow.WorkflowMapper;
import com.miletos.features.workflow.WorkflowService;
import com.miletos.features.workflow.controller.request.CreateWorkflowRequest;
import com.miletos.features.workflow.controller.request.UpdateWorkflowRequest;
import com.miletos.features.workflow.controller.response.WorkflowPageResponse;
import com.miletos.features.workflow.controller.response.WorkflowResponse;
import com.miletos.features.workflow.repository.entity.Workflow;
import com.miletos.features.workflow.repository.entity.WorkflowStatus;
import com.miletos.security.authorization.Authorize;
import com.miletos.security.authorization.RequiredRole;

import jakarta.validation.Valid;
import lombok.RequiredArgsConstructor;

@RestController
@RequestMapping("/api/workflows")
@RequiredArgsConstructor
public class WorkflowController {

        private final WorkflowService workflowService;
        private final WorkflowMapper workflowMapper;

        @Authorize(RequiredRole.COMPANY_ADMIN)
        @PostMapping
        public ResponseEntity<WorkflowResponse> createWorkflow(
                        @Valid @RequestBody CreateWorkflowRequest request) {
                Workflow workflow = workflowMapper.toEntity(request);
                Workflow createdWorkflow = workflowService.createWorkflow(workflow);
                return ResponseEntity.status(HttpStatus.CREATED)
                                .body(workflowMapper.toResponse(createdWorkflow));
        }

        @Authorize(RequiredRole.COMPANY_ADMIN)
        @GetMapping
        public ResponseEntity<WorkflowPageResponse> listWorkflows(
                        @RequestParam(defaultValue = "0") int page,
                        @RequestParam(defaultValue = "20") int size,
                        @RequestParam(required = false) String search,
                        @RequestParam(required = false) WorkflowStatus status) {
                Page<Workflow> workflows = workflowService.listWorkflows(page, size, search, status);
                return ResponseEntity.ok(workflowMapper.toPageResponse(workflows));
        }

        @Authorize(RequiredRole.COMPANY_ADMIN)
        @GetMapping("/{workflowId}")
        public ResponseEntity<WorkflowResponse> getWorkflow(
                        @PathVariable Long workflowId) {
                return ResponseEntity.ok(
                                workflowMapper.toResponse(workflowService.getWorkflow(workflowId)));
        }

        @Authorize(RequiredRole.COMPANY_ADMIN)
        @PutMapping("/{workflowId}")
        public ResponseEntity<WorkflowResponse> updateWorkflow(
                        @PathVariable Long workflowId,
                        @Valid @RequestBody UpdateWorkflowRequest request) {
                Workflow changes = workflowMapper.toEntity(request);
                return ResponseEntity.ok(
                                workflowMapper.toResponse(
                                                workflowService.updateWorkflow(workflowId, changes)));
        }

        @Authorize(RequiredRole.COMPANY_ADMIN)
        @DeleteMapping("/{workflowId}")
        public ResponseEntity<Void> deleteWorkflow(
                        @PathVariable Long workflowId) {
                workflowService.deleteWorkflow(workflowId);
                return ResponseEntity.noContent().build();
        }

        @Authorize(RequiredRole.COMPANY_ADMIN)
        @PostMapping("/{workflowId}/activate")
        public ResponseEntity<WorkflowResponse> activateWorkflow(
                        @PathVariable Long workflowId) {
                return ResponseEntity.ok(
                                workflowMapper.toResponse(
                                                workflowService.activateWorkflow(workflowId)));
        }

        @Authorize(RequiredRole.COMPANY_ADMIN)
        @PostMapping("/{workflowId}/archive")
        public ResponseEntity<WorkflowResponse> archiveWorkflow(
                        @PathVariable Long workflowId) {
                return ResponseEntity.ok(
                                workflowMapper.toResponse(workflowService.archiveWorkflow(workflowId)));
        }

        @Authorize(RequiredRole.COMPANY_ADMIN)
        @PostMapping("/{workflowId}/restore")
        public ResponseEntity<WorkflowResponse> restoreWorkflow(
                        @PathVariable Long workflowId) {
                return ResponseEntity.ok(
                                workflowMapper.toResponse(workflowService.restoreWorkflow(workflowId)));
        }
}
