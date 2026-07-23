import Icon from "@/components/lib/icon/Icon";

export type DashboardIconName =
  | "arrow"
  | "building"
  | "check"
  | "dashboard"
  | "key"
  | "mail"
  | "profile"
  | "shield"
  | "sparkles"
  | "team";

interface DashboardIconProps {
  name: DashboardIconName;
  size?: number;
}

const DashboardIcon = ({ name, size }: DashboardIconProps) => (
  <Icon src={`/icons/dashboard/${name}.svg`} size={size} />
);

export default DashboardIcon;
