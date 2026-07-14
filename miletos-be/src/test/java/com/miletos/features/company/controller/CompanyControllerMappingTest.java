package com.miletos.features.company.controller;

import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

import com.miletos.features.company.CompanyMapper;
import com.miletos.features.company.CompanyService;
import com.miletos.features.company.controller.request.InviteUserRequest;
import com.miletos.features.company.controller.response.InviteUserResponse;
import com.miletos.features.auth.repository.entity.AuthToken;
import java.util.List;
import org.junit.jupiter.api.Test;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.setup.MockMvcBuilders;

class CompanyControllerMappingTest {
    @Test
    void ownsCompanyUserListingAtExistingRoute() throws Exception {
        CompanyService service = mock(CompanyService.class);
        when(service.listCompanyUsers(null, 42L)).thenReturn(List.of());
        MockMvc mvc = MockMvcBuilders.standaloneSetup(new CompanyController(
                service, mock(CompanyMapper.class))).build();

        mvc.perform(get("/api/companies/42/users")).andExpect(status().isOk());

        verify(service).listCompanyUsers(null, 42L);
    }

    @Test
    void ownsCompanyInvitationAtPreservedAuthRoute() throws Exception {
        CompanyService service = mock(CompanyService.class);
        CompanyMapper companyMapper = mock(CompanyMapper.class);
        InviteUserRequest request = new InviteUserRequest(
                42L, "invitee@miletos.local", "Invite", "User",
                com.miletos.features.user.repository.entity.UserRole.USER);
        com.miletos.features.user.repository.entity.User invitedUser =
                mock(com.miletos.features.user.repository.entity.User.class);
        AuthToken token = mock(AuthToken.class);
        InviteUserResponse response = mock(InviteUserResponse.class);
        when(companyMapper.toInvitedUser(request)).thenReturn(invitedUser);
        when(service.inviteUser(null, 42L, invitedUser)).thenReturn(token);
        when(companyMapper.toInviteUserResponse(token)).thenReturn(response);
        MockMvc mvc = MockMvcBuilders.standaloneSetup(new CompanyController(
                service, companyMapper)).build();

        mvc.perform(org.springframework.test.web.servlet.request.MockMvcRequestBuilders
                        .post("/api/auth/invitations")
                        .contentType(org.springframework.http.MediaType.APPLICATION_JSON)
                        .content("""
                                {"companyId":42,"email":"invitee@miletos.local","firstName":"Invite","lastName":"User","role":"USER"}
                                """))
                .andExpect(status().isCreated());

        verify(service).inviteUser(null, 42L, invitedUser);
    }
}
