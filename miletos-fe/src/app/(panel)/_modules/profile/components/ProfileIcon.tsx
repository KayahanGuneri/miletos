import Icon from "@/components/lib/icon/Icon";

export type ProfileIconName =
  "camera" | "check" | "company" | "key" | "mail" | "profile" | "shield" | "upload";

interface ProfileIconProps {
  label?: string;
  name: ProfileIconName;
  size?: number;
}

const ProfileIcon = ({ label, name, size }: ProfileIconProps) => (
  <Icon label={label} size={size} src={`/icons/profile/${name}.svg`} />
);

export default ProfileIcon;
