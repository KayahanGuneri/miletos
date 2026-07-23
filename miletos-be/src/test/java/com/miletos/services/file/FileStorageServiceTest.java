package com.miletos.services.file;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

import com.miletos.services.file.metadata.StoredFile;
import com.miletos.services.file.metadata.StoredFileRepository;
import java.nio.file.Path;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

class FileStorageServiceTest {
    @TempDir
    Path storageRoot;

    @Test
    void storesAndLoadsGenericContentWithoutProductPolicy() {
        StoredFileRepository repository = mock(StoredFileRepository.class);
        when(repository.save(any())).thenAnswer(invocation -> invocation.getArgument(0));
        FileStorageService service = new FileStorageService(repository);

        StoredFile metadata = service.store(
                storageRoot, 7L, "../avatar.png", "image/png", "png", "content".getBytes());
        StoredContent loaded = service.load(storageRoot, metadata);

        assertThat(metadata.getOriginalFilename()).isEqualTo("avatar.png");
        assertThat(metadata.getStoragePath()).startsWith("7/");
        assertThat(loaded.content()).isEqualTo("content".getBytes());
        assertThat(loaded.contentType()).isEqualTo("image/png");
    }

    @Test
    void rejectsMetadataThatEscapesStorageRoot() {
        FileStorageService service = new FileStorageService(mock(StoredFileRepository.class));
        StoredFile metadata = new StoredFile(7L, "avatar.png", "image/png", "../outside.png", 1L);

        assertThatThrownBy(() -> service.load(storageRoot, metadata))
                .isInstanceOf(FileStorageException.class);
    }
}
