"use client";

import { Box } from "@/components/lib/box/Box";
import { useCurrentUserQuery } from "@/shared/session/hooks/useCurrentUserQuery";
import {
  canChangeOwnPassword,
  canUpdateOwnProfilePhoto,
} from "@/shared/session/permissions/session-permissions";
import PasswordChangeForm from "./components/PasswordChangeForm";
import ProfileAccountSummary from "./components/ProfileAccountSummary";
import ProfileHeader from "./components/ProfileHeader";
import ProfileIdentitySummary from "./components/ProfileIdentitySummary";
import ProfilePhotoForm from "./components/ProfilePhotoForm";
import { profileMessages } from "./messages/profile-messages";
import { useProfilePhotoQuery } from "./query/useProfilePhotoQuery";
import {
  formatProfileLabel,
  getProfileDisplayName,
  getProfileInitials,
} from "./utils/profile-display-utils";
import styles from "./ui/ProfilePagePanel.module.css";

const ProfilePage = () => {
  const currentUserQuery = useCurrentUserQuery();
  const user = currentUserQuery.data;
  const canChangePassword = canChangeOwnPassword(user);
  const canUpdateProfilePhoto = canUpdateOwnProfilePhoto(user);
  const profilePhotoQuery = useProfilePhotoQuery({
    enabled: canUpdateProfilePhoto && Boolean(user?.profilePhotoFileId),
  });
  const displayName = getProfileDisplayName(user?.firstName, user?.lastName);
  const roleLabel = user?.superAdmin ? "Superadmin" : formatProfileLabel(user?.role);

  return (
    <main className={styles.profilePage}>
      <Box className={styles.profilePage__shell}>
        <ProfileHeader />
        <ProfileIdentitySummary
          displayName={displayName}
          email={user?.email ?? profileMessages.common.emailUnavailable}
          initials={getProfileInitials(user?.firstName, user?.lastName)}
          photoUrl={profilePhotoQuery.profilePhotoUrl}
          roleLabel={roleLabel}
          statusLabel={formatProfileLabel(user?.status)}
        />
        <section className={styles.profilePage__contentGrid}>
          <ProfileAccountSummary
            companyId={user?.companyId}
            displayName={displayName}
            email={user?.email}
            onboardingLabel={formatProfileLabel(user?.onboardingStatus)}
            roleLabel={roleLabel}
            statusLabel={formatProfileLabel(user?.status)}
          />
          <div className={styles.profilePage__stack}>
            <ProfilePhotoForm canUpdate={canUpdateProfilePhoto} />
            <PasswordChangeForm canChange={canChangePassword} />
          </div>
        </section>
      </Box>
    </main>
  );
};

export default ProfilePage;
