import { profileMessages } from "../messages/profile-messages";
import styles from "../ui/ProfilePagePanel.module.css";

const ProfileHeader = () => (
  <header className={styles.profilePage__header}>
    <p className={styles.profilePage__eyebrow}>{profileMessages.page.eyebrow}</p>
    <h1 className={styles.profilePage__title}>{profileMessages.page.title}</h1>
    <p className={styles.profilePage__description}>{profileMessages.page.description}</p>
  </header>
);

export default ProfileHeader;
