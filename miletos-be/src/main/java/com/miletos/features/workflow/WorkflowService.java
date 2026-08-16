package com.miletos.features.workflow;

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
import com.miletos.features.workflow.service.WorkflowNodeSecretHandler;
import com.miletos.features.workflow.service.WorkflowRuntimeValidationGateway;
import com.miletos.features.workflow.service.WorkflowTriggerLifecycleGateway;
import lombok.RequiredArgsConstructor;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.PageRequest;
import org.springframework.data.domain.Sort;
import org.springframework.http.HttpHeaders;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

@Service
@RequiredArgsConstructor
public class WorkflowService {

  private static final Integer DEFAULT_PAGE_SIZE = 20;
  private static final Integer MAX_PAGE_SIZE = 100;

  private final WorkflowRepository workflowRepository;
  private final WorkflowMapper workflowMapper;
  private final WorkflowDefinitionPolicy workflowDefinitionPolicy;
  private final WorkflowNodeSecretHandler workflowNodeSecretHandler;
  private final WorkflowTriggerLifecycleGateway workflowTriggerLifecycleGateway;
  private final WorkflowRuntimeValidationGateway workflowRuntimeValidationGateway;

  @Transactional
  public Workflow createWorkflow(Workflow workflow, HttpHeaders browserHeaders) {
    Company company = workflow.getCompany();

    if (Boolean.TRUE.equals(
        workflowRepository.existsByCompanyAndNameIgnoreCase(company, workflow.getName()))) {
      throw new WorkflowNameAlreadyExistsException();
    }

    JsonNode definition =
        workflowDefinitionPolicy.validateAndNormalizeDefinition(workflow.getDefinitionJson());
    definition = workflowNodeSecretHandler.encryptOnSave(definition, null);

    workflowMapper.applyNormalizedDefinition(definition, workflow);

    Workflow saved = workflowRepository.saveAndFlush(workflow);
    workflowRuntimeValidationGateway.validateWorkflowDefinition(
        saved, company.getId(), browserHeaders);
    return saved;
  }

  @Transactional(readOnly = true)
  public Page<Workflow> listWorkflows(
      User user, Integer page, Integer size, String search, WorkflowStatus status) {

    Company company = user.getCompany();
    String normalizedSearch = normalizeSearch(search);

    PageRequest pageable =
        PageRequest.of(
            Math.max(page, 0),
            normalizePageSize(size),
            Sort.by(Sort.Order.desc("updatedAt"), Sort.Order.desc("id")));

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
  public Workflow updateWorkflow(Long workflowId, Workflow changes, HttpHeaders browserHeaders) {

    Company company = changes.getCompany();

    Workflow workflow = findOwnedWorkflow(workflowId, company);

    if (workflow.getStatus() != WorkflowStatus.DRAFT) {
      throw new WorkflowInvalidStateException();
    }

    if (Boolean.TRUE.equals(
        workflowRepository.existsByCompanyAndNameIgnoreCaseAndIdNot(
            company, changes.getName(), workflow.getId()))) {
      throw new WorkflowNameAlreadyExistsException();
    }

    JsonNode definition =
        workflowDefinitionPolicy.validateAndNormalizeDefinition(changes.getDefinitionJson());
    definition = workflowNodeSecretHandler.encryptOnSave(definition, workflow.getDefinitionJson());

    Long revision = workflow.getRevision() + 1L;

    workflowMapper.applyContentUpdate(changes, definition, revision, workflow);

    workflowRuntimeValidationGateway.validateWorkflowDefinition(
        workflow, company.getId(), browserHeaders);

    return workflowRepository.save(workflow);
  }

  @Transactional
  public Workflow activateWorkflow(User user, Long workflowId, HttpHeaders browserHeaders) {

    Company company = user.getCompany();

    Workflow workflow = findOwnedWorkflow(workflowId, company);

    if (workflow.getStatus() != WorkflowStatus.DRAFT) {
      throw new WorkflowInvalidStateException();
    }

    workflowRuntimeValidationGateway.validateWorkflowDefinition(
        workflow, company.getId(), browserHeaders);

    workflowTriggerLifecycleGateway.activateWorkflowSources(
        workflow, company.getId(), browserHeaders);

    try {
      workflowMapper.applyLifecycleUpdate(WorkflowStatus.ACTIVE, user, workflow);
      return workflowRepository.saveAndFlush(workflow);
    } catch (RuntimeException persistenceFailure) {
      try {
        workflowTriggerLifecycleGateway.disableWorkflowTriggers(
            workflow.getId(), company.getId(), browserHeaders);
      } catch (RuntimeException compensationFailure) {
        persistenceFailure.addSuppressed(compensationFailure);
      }
      throw persistenceFailure;
    }
  }

  @Transactional
  public Workflow archiveWorkflow(User user, Long workflowId, HttpHeaders browserHeaders) {

    Company company = user.getCompany();

    Workflow workflow = findOwnedWorkflow(workflowId, company);

    if (workflow.getStatus() != WorkflowStatus.ACTIVE) {
      throw new WorkflowInvalidStateException();
    }

    workflowTriggerLifecycleGateway.disableWorkflowTriggers(
        workflow.getId(), company.getId(), browserHeaders);

    workflowMapper.applyLifecycleUpdate(WorkflowStatus.ARCHIVED, user, workflow);

    return workflowRepository.save(workflow);
  }

  @Transactional
  public Workflow restoreWorkflow(User user, Long workflowId) {

    Workflow workflow = findOwnedWorkflow(workflowId, user.getCompany());

    if (workflow.getStatus() != WorkflowStatus.ARCHIVED) {
      throw new WorkflowInvalidStateException();
    }

    workflowMapper.applyLifecycleUpdate(WorkflowStatus.DRAFT, user, workflow);

    return workflowRepository.save(workflow);
  }

  @Transactional
  public void deleteWorkflow(User user, Long workflowId, HttpHeaders browserHeaders) {

    Company company = user.getCompany();

    Workflow workflow = findOwnedWorkflow(workflowId, company);

    if (workflow.getStatus() == WorkflowStatus.ACTIVE) {
      throw new WorkflowInvalidStateException();
    }

    workflowTriggerLifecycleGateway.disableWorkflowTriggers(
        workflow.getId(), company.getId(), browserHeaders);

    workflowRepository.delete(workflow);
    workflowRepository.flush();
  }

  private Workflow findOwnedWorkflow(Long workflowId, Company company) {

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
