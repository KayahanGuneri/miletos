package com.miletos.features.workflow.repository;

import java.util.Optional;

import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;
import org.springframework.data.jpa.repository.JpaRepository;

import com.miletos.features.workflow.repository.entity.Workflow;
import com.miletos.features.workflow.repository.entity.WorkflowStatus;

public interface WorkflowRepository extends JpaRepository<Workflow, Long> {

        Optional<Workflow> findByIdAndCompany_Id(Long id, Long companyId);

        boolean existsByCompany_IdAndNormalizedName(Long companyId, String normalizedName);

        boolean existsByCompany_IdAndNormalizedNameAndIdNot(
                        Long companyId, String normalizedName, Long id);

        Page<Workflow> findAllByCompany_Id(Long companyId, Pageable pageable);

        Page<Workflow> findAllByCompany_IdAndStatus(
                        Long companyId, WorkflowStatus status, Pageable pageable);

        Page<Workflow> findAllByCompany_IdAndNameContainingIgnoreCase(
                        Long companyId, String name, Pageable pageable);

        Page<Workflow> findAllByCompany_IdAndStatusAndNameContainingIgnoreCase(
                        Long companyId, WorkflowStatus status, String name, Pageable pageable);
}
