package com.miletos.features.workflow.repository;

import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;
import org.springframework.data.jpa.repository.JpaRepository;

import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.workflow.repository.entity.Workflow;
import com.miletos.features.workflow.repository.entity.WorkflowStatus;

public interface WorkflowRepository extends JpaRepository<Workflow, Long> {

    Boolean existsByCompanyAndNameIgnoreCase(Company company, String name);

    Boolean existsByCompanyAndNameIgnoreCaseAndIdNot(
            Company company, String name, Long id);

    Page<Workflow> findAllByCompany(Company company, Pageable pageable);

    Page<Workflow> findAllByCompanyAndStatus(
            Company company, WorkflowStatus status, Pageable pageable);

    Page<Workflow> findAllByCompanyAndNameContainingIgnoreCase(
            Company company, String name, Pageable pageable);

    Page<Workflow> findAllByCompanyAndStatusAndNameContainingIgnoreCase(
            Company company, WorkflowStatus status, String name, Pageable pageable);
}
