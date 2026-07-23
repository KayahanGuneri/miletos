import Image from "next/image";
import styles from "../ui/ProfilePagePanel.module.css";

interface ProfilePhotoDisplayProps {
  alt: string;
  initials: string;
  photoUrl: string | null;
}

const ProfilePhotoDisplay = ({ alt, initials, photoUrl }: ProfilePhotoDisplayProps) => {
  if (!photoUrl) {
    return (
      <span className={styles.profilePage__heroInitials} aria-hidden="true">
        {initials}
      </span>
    );
  }

  return (
    <Image
      alt={alt}
      className={styles.profilePage__heroAvatar}
      height={144}
      loading="lazy"
      src={photoUrl}
      unoptimized
      width={144}
    />
  );
};

export default ProfilePhotoDisplay;
