package com.miletos.features.user.repository;

import com.miletos.features.user.repository.entity.User;
import java.util.List;
import java.util.Optional;
import org.springframework.data.jpa.repository.JpaRepository;

public interface UserRepository extends JpaRepository<User, Long> {

    Optional<User> findByEmail(String email);

    boolean existsByEmail(String email);

    List<User> findAllByCompany_Id(Long companyId);

    Optional<User> findByIdAndCompany_Id(Long id, Long companyId);

    List<User> findAllByCompany_IdOrderByCreatedAtAsc(Long companyId);
}
