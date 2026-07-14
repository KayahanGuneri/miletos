import type { CSSProperties } from "react";
import styles from "./Icon.module.css";

interface IconProps {
  className?: string;
  label?: string;
  size?: number;
  src: string;
}

interface IconStyle extends CSSProperties {
  "--icon-size": string;
  "--icon-url": string;
}

const Icon = ({ className, label, size = 19, src }: IconProps) => {
  const iconStyle: IconStyle = {
    "--icon-size": `${size}px`,
    "--icon-url": `url("${src}")`,
  };

  const classNames = [styles.icon, className].filter(Boolean).join(" ");

  return (
    <span
      aria-hidden={label ? undefined : true}
      aria-label={label}
      role={label ? "img" : undefined}
      className={classNames}
      style={iconStyle}
    />
  );
};

export default Icon;
