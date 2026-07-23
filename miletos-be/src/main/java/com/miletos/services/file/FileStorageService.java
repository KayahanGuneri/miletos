package com.miletos.services.file;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.StandardOpenOption;
import java.util.UUID;

import org.springframework.stereotype.Service;

import com.miletos.services.file.metadata.StoredFile;
import com.miletos.services.file.metadata.StoredFileRepository;

import lombok.RequiredArgsConstructor;

@Service
@RequiredArgsConstructor
public class FileStorageService {
    private final StoredFileRepository storedFileRepository;

    public StoredFile store(Path storageRoot, Long ownerId, String originalFilename,
            String contentType, String extension, byte[] content) {
        String filename = UUID.randomUUID() + "." + extension;
        String relativePath = ownerId + "/" + filename;
        Path directory = storageRoot.resolve(ownerId.toString());
        try {
            Files.createDirectories(directory);
            Files.write(directory.resolve(filename), content,
                    StandardOpenOption.CREATE_NEW, StandardOpenOption.WRITE);
            return storedFileRepository.save(new StoredFile(
                    ownerId, normalizeFilename(originalFilename), contentType,
                    relativePath, content.length));
        } catch (IOException exception) {
            throw new FileStorageException(exception);
        }
    }

    public StoredContent load(Path storageRoot, StoredFile storedFile) {
        if (storedFile == null) {
            throw new StoredContentNotFoundException();
        }
        Path normalizedRoot = storageRoot.toAbsolutePath().normalize();
        Path target = normalizedRoot.resolve(storedFile.getStoragePath()).toAbsolutePath().normalize();
        if (!target.startsWith(normalizedRoot)) {
            throw new FileStorageException();
        }
        if (!Files.isRegularFile(target)) {
            throw new StoredContentNotFoundException();
        }
        try {
            return new StoredContent(Files.readAllBytes(target), storedFile.getContentType());
        } catch (IOException exception) {
            throw new FileStorageException(exception);
        }
    }

    private String normalizeFilename(String filename) {
        if (filename == null || filename.isBlank()) {
            return null;
        }
        String normalized = filename.trim().replace("\\", "/");
        normalized = normalized.substring(normalized.lastIndexOf('/') + 1);
        return normalized.length() > 255 ? normalized.substring(0, 255) : normalized;
    }
}
