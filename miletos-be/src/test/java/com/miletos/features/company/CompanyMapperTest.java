package com.miletos.features.company;

import static org.assertj.core.api.Assertions.assertThat;

import com.miletos.features.company.controller.request.CreateCompanyRequest;
import com.miletos.features.company.controller.request.InviteUserRequest;
import com.miletos.features.company.controller.request.UpdateUserNameRequest;
import com.miletos.features.company.repository.entity.Company;
import com.miletos.features.company.repository.entity.CompanyStatus;
import com.miletos.features.user.repository.entity.UserRole;
import org.junit.jupiter.api.Test;
import org.mapstruct.factory.Mappers;

class CompanyMapperTest {

    private final CompanyMapper mapper = Mappers.getMapper(CompanyMapper.class);

    @Test
    void mapsCreateRequestWithoutApplyingBusinessDefaults() {
        var company = mapper.toEntity(new CreateCompanyRequest("Acme"));

        assertThat(company.getName()).isEqualTo("Acme");
        assertThat(company.getStatus()).isNull();
    }

    @Test
    void mapsCanonicalCompanyStatusToResponse() {
        Company company = new Company("Acme", CompanyStatus.ACTIVE);

        var response = mapper.toResponse(company);

        assertThat(response.status()).isEqualTo(CompanyStatus.ACTIVE);
    }

    @Test
    void mapsCompanyAdministrationTransportWithoutApplyingBusinessPolicy() {
        var invitedUser = mapper.toInvitedUser(new InviteUserRequest(
                42L, " Invitee@Example.com ", " Invite ", " User ", UserRole.USER));
        var nameInput = mapper.toUserNameUpdateInput(
                "admin@example.com", 7L, new UpdateUserNameRequest(" Ada ", " Lovelace "));

        assertThat(invitedUser.getEmail()).isEqualTo(" Invitee@Example.com ");
        assertThat(invitedUser.getFirstName()).isEqualTo(" Invite ");
        assertThat(invitedUser.getStatus()).isNull();
        assertThat(nameInput.actorEmail()).isEqualTo("admin@example.com");
        assertThat(nameInput.targetUserId()).isEqualTo(7L);
        assertThat(nameInput.firstName()).isEqualTo(" Ada ");
    }
}
