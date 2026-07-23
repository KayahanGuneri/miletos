package com.miletos.features.user;

import static org.assertj.core.api.Assertions.assertThat;

import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.company.repository.entity.CompanyStatus;
import com.miletos.features.user.repository.entity.OnboardingStatus;
import com.miletos.features.user.repository.entity.User;
import com.miletos.features.user.repository.entity.UserRole;
import com.miletos.features.user.repository.entity.UserStatus;
import org.junit.jupiter.api.Test;
import org.mapstruct.factory.Mappers;

class UserMapperTest {

    private final UserMapper mapper = Mappers.getMapper(UserMapper.class);

    @Test
    void mapsNestedIdentifiersAndEnumNames() {
        Company company = new Company("Acme", CompanyStatus.ACTIVE);
        company.setId(41L);
        User user = new User(
                null,
                company,
                "user@example.com",
                "hash",
                "Ada",
                "Lovelace",
                UserRole.MOD,
                false,
                UserStatus.ACTIVE,
                OnboardingStatus.COMPLETED,
                null,
                null,
                null,
                null
        );
        user.setId(7L);

        var response = mapper.toResponse(user);

        assertThat(response.id()).isEqualTo(7L);
        assertThat(response.companyId()).isEqualTo(41L);
        assertThat(response.role()).isEqualTo("MOD");
        assertThat(response.status()).isEqualTo("ACTIVE");
        assertThat(response.onboardingStatus()).isEqualTo("COMPLETED");
        assertThat(response.profilePhotoFileId()).isNull();
    }
}
