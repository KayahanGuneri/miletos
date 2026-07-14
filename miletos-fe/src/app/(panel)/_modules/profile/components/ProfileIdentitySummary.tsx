import Image from "next/image";
import ProfileIcon from "./ProfileIcon";
import ProfilePhotoDisplay from "./ProfilePhotoDisplay";
import styles from "../ui/ProfilePagePanel.module.css";

interface ProfileIdentitySummaryProps {
  displayName: string;
  email: string;
  initials: string;
  photoUrl: string | null;
  roleLabel: string;
  statusLabel: string;
}

const ProfileIdentitySummary = ({
  displayName,
  email,
  initials,
  photoUrl,
  roleLabel,
  statusLabel,
}: ProfileIdentitySummaryProps) => (
  <section className={styles.profilePage__hero}>
    <div className={styles.profilePage__identityBlock}>
      <div className={styles.profilePage__avatarFrame}>
        <ProfilePhotoDisplay
          alt={`${displayName} profile photo`}
          initials={initials}
          photoUrl={photoUrl}
        />
        <span className={styles.profilePage__avatarStatus} aria-label="Active account">
          <ProfileIcon name="check" size={14} />
        </span>
      </div>

      <div className={styles.profilePage__identityCopy}>
        <div className={styles.profilePage__identityBadges}>
          <span>
            <ProfileIcon name="shield" size={15} />
            {roleLabel}
          </span>
          <span>
            <ProfileIcon name="check" size={15} />
            {statusLabel}
          </span>
        </div>
        <h2>{displayName}</h2>
        <p>
          <ProfileIcon name="mail" size={17} />
          {email}
        </p>
        <small>Your visible identity is connected to the current authenticated session.</small>
      </div>
    </div>

    <div className={styles.profilePage__heroVisual}>
      <div className={styles.profilePage__heroVisualCopy}>
        <p>Account protection</p>
        <strong>Keep credentials and profile assets aligned with your access model.</strong>
        <span>
          Security, identity and workflow access remain connected inside one protected workspace.
        </span>
      </div>
      <div className={styles.profilePage__heroImageStage}>
        <Image
          alt="Account security workflow illustration"
          className={styles.profilePage__heroImage}
          height={941}
          priority
          src="/images/illustrations/dashboard-workflow.png"
          width={1672}
        />
      </div>
    </div>
  </section>
);

export default ProfileIdentitySummary;
