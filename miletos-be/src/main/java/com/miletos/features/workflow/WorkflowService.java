package com.miletos.features.workflow;

import java.util.Locale;

import org.springframework.data.domain.Page;
import org.springframework.data.domain.PageRequest;
import org.springframework.data.domain.Sort;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import org.hibernate.exception.ConstraintViolationException;

import com.fasterxml.jackson.databind.JsonNode;
import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.workflow.exception.InvalidWorkflowDescriptionException;
import com.miletos.features.workflow.exception.InvalidWorkflowNameException;
import com.miletos.features.workflow.exception.WorkflowInvalidStateException;
import com.miletos.features.workflow.exception.WorkflowNameAlreadyExistsException;
import com.miletos.features.workflow.exception.WorkflowNotFoundException;
import com.miletos.features.workflow.repository.WorkflowRepository;
import com.miletos.features.workflow.repository.entity.Workflow;
import com.miletos.features.workflow.repository.entity.WorkflowStatus;
import com.miletos.features.workflow.service.WorkflowDefinitionPolicy;
import com.miletos.security.AuthenticatedActorResolver;
import com.miletos.security.authorization.AuthorizationEvaluator;
import com.miletos.security.exception.OperationNotAuthorizedException;

import lombok.RequiredArgsConstructor;

@Service
@RequiredArgsConstructor
public class WorkflowService {

    private static final int DEFAULT_PAGE_SIZE = 20;
    private static final int MAX_PAGE_SIZE = 100;
    private static final String UNIQUE_WORKFLOW_NAME_CONSTRAINT =
            "uk_workflows_company_normalized_name";

    private final WorkflowRepository workflowRepository;
    private final AuthenticatedActorResolver authenticatedActorResolver;
    private final AuthorizationEvaluator authorizationEvaluator;
    private final WorkflowDefinitionPolicy workflowDefinitionPolicy;

    @Transactional
    public Workflow createWorkflow(Workflow changes) {
        User actor = resolveCurrentCompanyAdmin();
        Company company = actor.getCompany();
        String name = normalizeName(changes.getName());
        String normalizedName = name.toLowerCase(Locale.ROOT);
        String description = normalizeDescription(changes.getDescription());

        if (workflowRepository.existsByCompany_IdAndNormalizedName(
                company.getId(), normalizedName)) {
            throw new WorkflowNameAlreadyExistsException();
        }

        JsonNode definition = workflowDefinitionPolicy.validateAndNormalizeDefinition(
                changes.getDefinitionJson());
        Workflow workflow = new Workflow(
                name,
                description,
                definition);
        workflow.setCompany(company);
        workflow.setNormalizedName(normalizedName);
        workflow.setStatus(WorkflowStatus.DRAFT);
        workflow.setRevision(1);
        workflow.setCreatedByEmail(actor.getEmail());
        workflow.setUpdatedByEmail(actor.getEmail());
        return saveAndFlushTranslatingDuplicateName(workflow);
    }

    @Transactional(readOnly = true)
    public Page<Workflow> listWorkflows(
            int page,
            int size,
            String search,
            WorkflowStatus status) {
        User actor = resolveCurrentCompanyAdmin();
        Long companyId = actor.getCompany().getId();
        String normalizedSearch = normalizeSearch(search);
        PageRequest pageable = PageRequest.of(
                Math.max(page, 0),
                normalizePageSize(size),
                Sort.by(
                        Sort.Order.desc("updatedAt"),
                        Sort.Order.desc("id")));

        if (status != null && normalizedSearch != null) {
            return workflowRepository.findAllByCompany_IdAndStatusAndNameContainingIgnoreCase(
                    companyId, status, normalizedSearch, pageable);
        }
        if (status != null) {
            return workflowRepository.findAllByCompany_IdAndStatus(companyId, status, pageable);
        }
        if (normalizedSearch != null) {
            return workflowRepository.findAllByCompany_IdAndNameContainingIgnoreCase(
                    companyId, normalizedSearch, pageable);
        }
        return workflowRepository.findAllByCompany_Id(companyId, pageable);
    }

    @Transactional(readOnly = true)
    public Workflow getWorkflow(Long workflowId) {
        User actor = resolveCurrentCompanyAdmin();
        return findWorkflow(workflowId, actor.getCompany().getId());
    }

    @Transactional
    public Workflow updateWorkflow(Long workflowId, Workflow changes) {
        User actor = resolveCurrentCompanyAdmin();
        Long companyId = actor.getCompany().getId();
        Workflow workflow = findWorkflow(workflowId, companyId);
        requireDraft(workflow);

        String name = normalizeName(changes.getName());
        String normalizedName = name.toLowerCase(Locale.ROOT);
        String description = normalizeDescription(changes.getDescription());
        if (workflowRepository.existsByCompany_IdAndNormalizedNameAndIdNot(
                companyId, normalizedName, workflow.getId())) {
            throw new WorkflowNameAlreadyExistsException();
        }

        JsonNode definition = workflowDefinitionPolicy.validateAndNormalizeDefinition(
                changes.getDefinitionJson());
        workflow.setName(name);
        workflow.setNormalizedName(normalizedName);
        workflow.setDescription(description);
        workflow.setDefinitionJson(definition);
        workflow.setRevision(workflow.getRevision() + 1);
        workflow.setUpdatedByEmail(actor.getEmail());
        return saveAndFlushTranslatingDuplicateName(workflow);
    }

    @Transactional
    public Workflow activateWorkflow(Long workflowId) {
        User actor = resolveCurrentCompanyAdmin();
        Workflow workflow = findWorkflow(workflowId, actor.getCompany().getId());
        requireStatus(workflow, WorkflowStatus.DRAFT);
        workflow.setStatus(WorkflowStatus.ACTIVE);
        workflow.setUpdatedByEmail(actor.getEmail());
        return workflowRepository.save(workflow);
    }

    @Transactional
    public Workflow archiveWorkflow(Long workflowId) {
        User actor = resolveCurrentCompanyAdmin();
        Workflow workflow = findWorkflow(workflowId, actor.getCompany().getId());
        requireStatus(workflow, WorkflowStatus.ACTIVE);
        workflow.setStatus(WorkflowStatus.ARCHIVED);
        workflow.setUpdatedByEmail(actor.getEmail());
        return workflowRepository.save(workflow);
    }

    @Transactional
    public Workflow restoreWorkflow(Long workflowId) {
        User actor = resolveCurrentCompanyAdmin();
        Workflow workflow = findWorkflow(workflowId, actor.getCompany().getId());
        requireStatus(workflow, WorkflowStatus.ARCHIVED);
        workflow.setStatus(WorkflowStatus.DRAFT);
        workflow.setUpdatedByEmail(actor.getEmail());
        return workflowRepository.save(workflow);
    }

    @Transactional
    public void deleteWorkflow(Long workflowId) {
        User actor = resolveCurrentCompanyAdmin();
        Workflow workflow = findWorkflow(workflowId, actor.getCompany().getId());
        if (workflow.getStatus() == WorkflowStatus.ACTIVE) {
            throw new WorkflowInvalidStateException();
        }
        workflowRepository.delete(workflow);
        workflowRepository.flush();
    }

    private User resolveCurrentCompanyAdmin() {
        User actor = authenticatedActorResolver.resolveCurrent();
        if (!authorizationEvaluator.isCompanyAdmin(actor)) {
            throw new OperationNotAuthorizedException();
        }
        return actor;
    }

    private Workflow saveAndFlushTranslatingDuplicateName(Workflow workflow) {
        try {
            return workflowRepository.saveAndFlush(workflow);
        } catch (DataIntegrityViolationException exception) {
            if (hasConstraint(exception, UNIQUE_WORKFLOW_NAME_CONSTRAINT)) {
                throw new WorkflowNameAlreadyExistsException();
            }
            throw exception;
        }
    }

    private boolean hasConstraint(Throwable exception, String constraintName) {
        Throwable cause = exception;
        while (cause != null) {
            if (cause instanceof ConstraintViolationException constraintViolation
                    && constraintName.equals(constraintViolation.getConstraintName())) {
                return true;
            }
            cause = cause.getCause();
        }
        return false;
    }

    private Workflow findWorkflow(Long workflowId, Long companyId) {
        return workflowRepository
                .findByIdAndCompany_Id(workflowId, companyId)
                .orElseThrow(WorkflowNotFoundException::new);
    }

    private void requireDraft(Workflow workflow) {
        requireStatus(workflow, WorkflowStatus.DRAFT);
    }

    private void requireStatus(Workflow workflow, WorkflowStatus requiredStatus) {
        if (workflow.getStatus() != requiredStatus) {
            throw new WorkflowInvalidStateException();
        }
    }

    private String normalizeName(String name) {
        if (name == null || name.isBlank()) {
            throw new InvalidWorkflowNameException();
        }
        String normalized = name.trim();
        if (normalized.length() < 3 || normalized.length() > 120) {
            throw new InvalidWorkflowNameException();
        }
        return normalized;
    }

    private String normalizeDescription(String description) {
        if (description == null) {
            return null;
        }
        String normalized = description.trim();
        if (normalized.isBlank()) {
            return null;
        }
        if (normalized.length() > 1000) {
            throw new InvalidWorkflowDescriptionException();
        }
        return normalized;
    }

    private String normalizeSearch(String search) {
        if (search == null || search.isBlank()) {
            return null;
        }
        return search.trim();
    }

    private int normalizePageSize(int size) {
        if (size <= 0) {
            return DEFAULT_PAGE_SIZE;
        }
        return Math.min(size, MAX_PAGE_SIZE);
    }
}
