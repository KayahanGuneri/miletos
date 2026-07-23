import ProfileIcon from "./ProfileIcon";
import { profileMessages } from "../messages/profile-messages";
import styles from "../ui/ProfilePagePanel.module.css";

interface ProfileAccountSummaryProps {
  companyId: number | string | null | undefined;
  displayName: string;
  email: string | null | undefined;
  onboardingLabel: string;
  roleLabel: string;
  statusLabel: string;
}

const ProfileAccountSummary = ({
  companyId,
  displayName,
  email,
  onboardingLabel,
  roleLabel,
  statusLabel,
}: ProfileAccountSummaryProps) => (
  <article className={styles.profilePage__card}>
    <div className={styles.profilePage__cardHeader}>
      <span className={styles.profilePage__cardIcon}>
        <ProfileIcon name="profile" />
      </span>
      <div>
        <p className={styles.profilePage__cardEyebrow}>Identity details</p>
        <h2 className={styles.profilePage__cardTitle}>Account overview</h2>
      </div>
    </div>

    <dl className={styles.profilePage__details}>
      <div className={styles.profilePage__detailRow}>
        <dt>
          <ProfileIcon name="mail" size={17} />
          Email
        </dt>
        <dd>{email ?? profileMessages.common.notAssigned}</dd>
      </div>
      <div className={styles.profilePage__detailRow}>
        <dt>
          <ProfileIcon name="profile" size={17} />
          Full name
        </dt>
        <dd>{displayName}</dd>
      </div>
      <div className={styles.profilePage__detailRow}>
        <dt>
          <ProfileIcon name="shield" size={17} />
          Role
        </dt>
        <dd>
          <span className={styles.profilePage__badge}>{roleLabel}</span>
        </dd>
      </div>
      <div className={styles.profilePage__detailRow}>
        <dt>
          <ProfileIcon name="check" size={17} />
          Status
        </dt>
        <dd>{statusLabel}</dd>
      </div>
      <div className={styles.profilePage__detailRow}>
        <dt>
          <ProfileIcon name="key" size={17} />
          Onboarding
        </dt>
        <dd>{onboardingLabel}</dd>
      </div>
      <div className={styles.profilePage__detailRow}>
        <dt>
          <ProfileIcon name="company" size={17} />
          Company ID
        </dt>
        <dd title={companyId != null ? String(companyId) : undefined}>
          {companyId ?? profileMessages.common.notAssigned}
        </dd>
      </div>
    </dl>

    <div className={styles.profilePage__securityNote}>
      <ProfileIcon name="shield" size={22} />
      <div>
        <strong>Protected account context</strong>
        <span>Permissions are resolved from your active role and company membership.</span>
      </div>
    </div>
  </article>
);

export default ProfileAccountSummary;
