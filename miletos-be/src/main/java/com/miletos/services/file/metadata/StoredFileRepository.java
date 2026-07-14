package com.miletos.services.file.metadata;

import java.util.List;

import org.springframework.data.jpa.repository.JpaRepository;

public interface StoredFileRepository extends JpaRepository<StoredFile, Long> {

    List<StoredFile> findAllByOwnerUserId(Long ownerUserId);
}
