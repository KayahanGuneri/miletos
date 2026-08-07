package com.miletos.features.workflow;

import org.springframework.data.domain.Page;
import org.springframework.data.domain.PageRequest;
import org.springframework.data.domain.Sort;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import com.fasterxml.jackson.databind.JsonNode;
import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.workflow.exception.WorkflowInvalidStateException;
import com.miletos.features.workflow.exception.WorkflowNameAlreadyExistsException;
import com.miletos.features.workflow.exception.WorkflowNotFoundException;
import com.miletos.features.workflow.repository.WorkflowRepository;
import com.miletos.features.workflow.repository.entity.Workflow;
import com.miletos.features.workflow.repository.entity.WorkflowStatus;
import com.miletos.features.workflow.service.WorkflowDefinitionPolicy;

import lombok.RequiredArgsConstructor;

@Service
@RequiredArgsConstructor
public class WorkflowService {

    private static final Integer DEFAULT_PAGE_SIZE = 20;
    private static final Integer MAX_PAGE_SIZE = 100;

    private final WorkflowRepository workflowRepository;
    private final WorkflowMapper workflowMapper;
    private final WorkflowDefinitionPolicy workflowDefinitionPolicy;

    @Transactional
    public Workflow createWorkflow(Workflow workflow) {
        Company company = workflow.getCompany();

        if (Boolean.TRUE.equals(workflowRepository.existsByCompanyAndNameIgnoreCase(
                company, workflow.getName()))) {
            throw new WorkflowNameAlreadyExistsException();
        }

        JsonNode definition = workflowDefinitionPolicy.validateAndNormalizeDefinition(
                workflow.getDefinitionJson());
        workflowMapper.applyNormalizedDefinition(definition, workflow);

        return workflowRepository.save(workflow);
    }

    @Transactional(readOnly = true)
    public Page<Workflow> listWorkflows(
            User user,
            Integer page,
            Integer size,
            String search,
            WorkflowStatus status) {
        Company company = user.getCompany();
        String normalizedSearch = normalizeSearch(search);
        PageRequest pageable = PageRequest.of(
                Math.max(page, 0),
                normalizePageSize(size),
                Sort.by(
                        Sort.Order.desc("updatedAt"),
                        Sort.Order.desc("id")));

        if (status == null) {
            return normalizedSearch == null
                    ? workflowRepository.findAllByCompany(company, pageable)
                    : workflowRepository.findAllByCompanyAndNameContainingIgnoreCase(
                            company, normalizedSearch, pageable);
        }

        return normalizedSearch == null
                ? workflowRepository.findAllByCompanyAndStatus(company, status, pageable)
                : workflowRepository.findAllByCompanyAndStatusAndNameContainingIgnoreCase(
                        company, status, normalizedSearch, pageable);
    }

    @Transactional(readOnly = true)
    public Workflow getWorkflowById(User user, Long workflowId) {
        return findOwnedWorkflow(workflowId, user.getCompany());
    }

    @Transactional
    public Workflow updateWorkflow(Long workflowId, Workflow changes) {
        Company company = changes.getCompany();
        Workflow workflow = findOwnedWorkflow(workflowId, company);
        if (workflow.getStatus() != WorkflowStatus.DRAFT) {
            throw new WorkflowInvalidStateException();
        }

        if (Boolean.TRUE.equals(workflowRepository.existsByCompanyAndNameIgnoreCaseAndIdNot(
                company, changes.getName(), workflow.getId()))) {
            throw new WorkflowNameAlreadyExistsException();
        }

        JsonNode definition = workflowDefinitionPolicy.validateAndNormalizeDefinition(
                changes.getDefinitionJson());
        Long revision = workflow.getRevision() + 1L;
        workflowMapper.applyContentUpdate(
                changes,
                definition,
                revision,
                workflow);

        return workflowRepository.save(workflow);
    }

    @Transactional
    public Workflow activateWorkflow(User user, Long workflowId) {
        Workflow workflow = findOwnedWorkflow(workflowId, user.getCompany());
        if (workflow.getStatus() != WorkflowStatus.DRAFT) {
            throw new WorkflowInvalidStateException();
        }

        workflowMapper.applyLifecycleUpdate(
                WorkflowStatus.ACTIVE,
                user,
                workflow);
        return workflowRepository.save(workflow);
    }

    @Transactional
    public Workflow archiveWorkflow(User user, Long workflowId) {
        Workflow workflow = findOwnedWorkflow(workflowId, user.getCompany());
        if (workflow.getStatus() != WorkflowStatus.ACTIVE) {
            throw new WorkflowInvalidStateException();
        }

        workflowMapper.applyLifecycleUpdate(
                WorkflowStatus.ARCHIVED,
                user,
                workflow);
        return workflowRepository.save(workflow);
    }

    @Transactional
    public Workflow restoreWorkflow(User user, Long workflowId) {
        Workflow workflow = findOwnedWorkflow(workflowId, user.getCompany());
        if (workflow.getStatus() != WorkflowStatus.ARCHIVED) {
            throw new WorkflowInvalidStateException();
        }

        workflowMapper.applyLifecycleUpdate(
                WorkflowStatus.DRAFT,
                user,
                workflow);
        return workflowRepository.save(workflow);
    }

    @Transactional
    public void deleteWorkflow(User user, Long workflowId) {
        Workflow workflow = findOwnedWorkflow(workflowId, user.getCompany());
        if (workflow.getStatus() == WorkflowStatus.ACTIVE) {
            throw new WorkflowInvalidStateException();
        }

        workflowRepository.delete(workflow);
        workflowRepository.flush();
    }

    private Workflow findOwnedWorkflow(
            Long workflowId,
            Company company) {
        return workflowRepository
                .findByIdAndCompany(workflowId, company)
                .orElseThrow(WorkflowNotFoundException::new);
    }

    private String normalizeSearch(String search) {
        if (search == null || search.isBlank()) {
            return null;
        }
        return search.trim();
    }

    private Integer normalizePageSize(Integer size) {
        if (size == null || size <= 0) {
            return DEFAULT_PAGE_SIZE;
        }
        return Math.min(size, MAX_PAGE_SIZE);
    }
}
