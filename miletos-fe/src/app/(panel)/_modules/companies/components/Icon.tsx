import Icon from "@/components/lib/icon/Icon";

export type IconName =
  | "arrow-left"
  | "arrow-right"
  | "building"
  | "check"
  | "clock"
  | "edit"
  | "mail"
  | "plus"
  | "refresh"
  | "search"
  | "send"
  | "shield"
  | "team"
  | "trash"
  | "user";

interface IconProps {
  className?: string;
  label?: string;
  name: IconName;
  size?: number;
}

const CompanyModuleIcon = ({ className, label, name, size }: IconProps) => (
  <Icon className={className} label={label} size={size} src={`/icons/company/${name}.svg`} />
);

export default CompanyModuleIcon;
