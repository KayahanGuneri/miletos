package com.miletos.features.workflow.repository;

import java.util.Optional;

import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;
import org.springframework.data.jpa.repository.EntityGraph;
import org.springframework.data.jpa.repository.JpaRepository;

import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.workflow.repository.entity.Workflow;
import com.miletos.features.workflow.repository.entity.WorkflowStatus;

public interface WorkflowRepository extends JpaRepository<Workflow, Long> {

    Boolean existsByCompanyAndNameIgnoreCase(Company company, String name);

    Boolean existsByCompanyAndNameIgnoreCaseAndIdNot(
            Company company, String name, Long id);

    @EntityGraph(attributePaths = { "createdBy", "updatedBy" })
    Optional<Workflow> findByIdAndCompany(Long id, Company company);

    @EntityGraph(attributePaths = { "createdBy", "updatedBy" })
    Page<Workflow> findAllByCompany(Company company, Pageable pageable);

    @EntityGraph(attributePaths = { "createdBy", "updatedBy" })
    Page<Workflow> findAllByCompanyAndStatus(
            Company company, WorkflowStatus status, Pageable pageable);

    @EntityGraph(attributePaths = { "createdBy", "updatedBy" })
    Page<Workflow> findAllByCompanyAndNameContainingIgnoreCase(
            Company company, String name, Pageable pageable);

    @EntityGraph(attributePaths = { "createdBy", "updatedBy" })
    Page<Workflow> findAllByCompanyAndStatusAndNameContainingIgnoreCase(
            Company company, WorkflowStatus status, String name, Pageable pageable);
}
