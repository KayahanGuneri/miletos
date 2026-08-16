package com.miletos.features.workflow.service;

import com.miletos.config.WorkflowInputProperties;
import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.workflow.exception.InvalidWorkflowInputFileException;
import com.miletos.features.workflow.exception.WorkflowInputFileStorageException;
import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Locale;
import java.util.Set;
import java.util.UUID;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.web.multipart.MultipartFile;

@Service
@RequiredArgsConstructor
public class WorkflowInputFileStorageService {

  private static final Set<String> ALLOWED_EXTENSIONS = Set.of(".txt", ".csv", ".xlsx");

  private final WorkflowInputProperties workflowInputProperties;

  public String store(User user, MultipartFile file) {
    if (file == null || file.isEmpty()) {
      throw new InvalidWorkflowInputFileException();
    }

    Company company = user.getCompany();
    if (company == null || company.getId() == null) {
      throw new InvalidWorkflowInputFileException();
    }

    String originalFilename = file.getOriginalFilename();
    String sanitizedOriginalName = sanitizeFileName(originalFilename);
    if (sanitizedOriginalName == null || !hasAllowedExtension(sanitizedOriginalName)) {
      throw new InvalidWorkflowInputFileException();
    }

    String storedFileName = UUID.randomUUID() + "_" + sanitizedOriginalName;

    Path storageRoot =
        Path.of(workflowInputProperties.inputDirectory()).toAbsolutePath().normalize();
    Path companyDirectory = storageRoot.resolve(company.getId().toString()).normalize();
    if (!companyDirectory.startsWith(storageRoot)) {
      throw new InvalidWorkflowInputFileException();
    }

    Path target = companyDirectory.resolve(storedFileName).normalize();
    if (!target.startsWith(companyDirectory)
        || !target.getFileName().toString().equals(storedFileName)) {
      throw new InvalidWorkflowInputFileException();
    }

    try {
      Files.createDirectories(companyDirectory);
      try (var inputStream = file.getInputStream()) {
        Files.copy(inputStream, target);
      }
      return storedFileName;
    } catch (IOException exception) {
      throw new WorkflowInputFileStorageException(exception);
    }
  }

  private String sanitizeFileName(String filename) {
    if (filename == null || filename.isBlank()) {
      return null;
    }
    String normalized = filename.trim().replace('\\', '/');
    int separator = normalized.lastIndexOf('/');
    if (separator >= 0) {
      normalized = normalized.substring(separator + 1);
    }
    if (normalized.isBlank()
        || normalized.contains("..")
        || normalized.contains("/")
        || normalized.contains("\\")
        || normalized.contains(":")) {
      return null;
    }
    return normalized;
  }

  private boolean hasAllowedExtension(String fileName) {
    String lower = fileName.toLowerCase(Locale.ROOT);
    return ALLOWED_EXTENSIONS.stream().anyMatch(lower::endsWith);
  }
}
