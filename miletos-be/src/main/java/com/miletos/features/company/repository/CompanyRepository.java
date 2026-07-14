package com.miletos.features.company.repository;

import java.util.Optional;

import org.springframework.data.jpa.repository.JpaRepository;

import com.miletos.features.company.repository.entity.Company;

public interface CompanyRepository extends JpaRepository<Company, Long> {

    boolean existsByNameIgnoreCase(String name);

    boolean existsByNameIgnoreCaseAndIdNot(String name, Long id);

    Optional<Company> findByNameIgnoreCase(String name);
}