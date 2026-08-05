package com.miletos.features.workflow.repository;

import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.workflow.repository.entity.Workflow;
import com.miletos.features.workflow.repository.entity.WorkflowStatus;

public interface WorkflowRepository extends JpaRepository<Workflow, Long> {

    Boolean existsByCompanyAndNameIgnoreCase(Company company, String name);

    Boolean existsByCompanyAndNameIgnoreCaseAndIdNot(
            Company company, String name, Long id);

    @Query("""
            select workflow
            from Workflow workflow
            where workflow.company = :company
              and (:status is null or workflow.status = :status)
              and lower(workflow.name) like lower(concat('%', :search, '%'))
            """)
    Page<Workflow> findAllByFilters(
            @Param("company") Company company,
            @Param("status") WorkflowStatus status,
            @Param("search") String search,
            Pageable pageable);
}
