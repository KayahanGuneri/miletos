"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { type ReactNode, useEffect, useState, useSyncExternalStore } from "react";
import { Box } from "@/components/lib/box/Box";
import GenericIcon from "@/components/lib/icon/Icon";
import { SESSION_UNAUTHORIZED_EVENT } from "@/shared/session/events/session-events";
import {
  canAccessCompanyUsers,
  canCreateCompany,
  canManageWorkflows,
} from "@/shared/session/permissions/session-permissions";
import { hasAccessToken } from "@/shared/session/storage/access-token-storage";
import { type UserProfile } from "@/shared/session/types/session-user-types";
import { useCurrentUserQuery } from "@/shared/session/hooks/useCurrentUserQuery";
import { useLogout } from "@/shared/session/hooks/useLogout";
import { PanelUserIdentity } from "./PanelUserIdentity";
import styles from "./PanelGuard.module.css";

interface PanelGuardProps {
  children: ReactNode;
}

interface AuthenticatedPanelShellProps {
  children: ReactNode;
  pathname: string;
  user: UserProfile;
  onLogout: () => void;
}

type NavigationIcon =
  | "building"
  | "dashboard"
  | "executions"
  | "logout"
  | "menu"
  | "profile"
  | "shield"
  | "users"
  | "workflows"
  | "x";

interface IconProps {
  name: NavigationIcon;
  size?: number;
}

interface NavigationItem {
  href: string;
  label: string;
  description: string;
  icon: NavigationIcon;
  active: boolean;
}

function subscribeToClientReady() {
  return () => {};
}

function getClientSnapshot() {
  return true;
}

function getServerSnapshot() {
  return false;
}

function Icon({ name, size = 20 }: IconProps) {
  const sources: Record<NavigationIcon, string> = {
    building: "/icons/dashboard/building.svg",
    dashboard: "/icons/dashboard/dashboard.svg",
    executions: "/icons/dashboard/sparkles.svg",
    logout: "/icons/navigation/logout.svg",
    menu: "/icons/navigation/menu.svg",
    profile: "/icons/dashboard/profile.svg",
    shield: "/icons/dashboard/shield.svg",
    users: "/icons/dashboard/team.svg",
    workflows: "/icons/dashboard/sparkles.svg",
    x: "/icons/navigation/x.svg",
  };
  return <GenericIcon size={size} src={sources[name]} />;
}

function getCurrentPageTitle(pathname: string) {
  if (pathname.startsWith("/admin/companies/") && pathname.endsWith("/users")) {
    return "Company users";
  }

  if (pathname === "/admin/companies") {
    return "Company management";
  }

  if (pathname === "/profile") {
    return "Profile & security";
  }

  if (pathname === "/dashboard/executions") {
    return "Workflow executions";
  }

  if (pathname.startsWith("/dashboard/executions/")) {
    return "Execution detail";
  }

  if (pathname === "/workflows") {
    return "Workflow management";
  }

  if (pathname === "/workflows/new") {
    return "New workflow";
  }

  if (pathname.startsWith("/workflows/")) {
    return "Workflow editor";
  }

  return "Dashboard";
}

function buildNavigationItems(pathname: string, user: UserProfile): NavigationItem[] {
  const items: NavigationItem[] = [
    {
      href: "/dashboard",
      label: "Dashboard",
      description: "Workspace overview",
      icon: "dashboard",
      active: pathname === "/dashboard",
    },
  ];

  items.push({
    href: "/dashboard/executions",
    label: "Executions",
    description: "Workflow runtime history",
    icon: "executions",
    active: pathname === "/dashboard/executions" || pathname.startsWith("/dashboard/executions/"),
  });

  if (canManageWorkflows(user)) {
    items.push({
      href: "/workflows",
      label: "Workflows",
      description: "Design and lifecycle",
      icon: "workflows",
      active: pathname === "/workflows" || pathname.startsWith("/workflows/"),
    });
  }

  if (canCreateCompany(user)) {
    items.push({
      href: "/admin/companies",
      label: "Companies",
      description: "Tenant management",
      icon: "building",
      active: pathname === "/admin/companies",
    });
  }

  if (
    !user.superAdmin &&
    user.role === "ADMIN" &&
    user.companyId &&
    canAccessCompanyUsers(user, user.companyId)
  ) {
    const companyUsersHref = `/admin/companies/${user.companyId}/users`;

    items.push({
      href: companyUsersHref,
      label: "Company users",
      description: "Members and invitations",
      icon: "users",
      active: pathname === companyUsersHref,
    });
  }

  items.push({
    href: "/profile",
    label: "Profile",
    description: "Identity and security",
    icon: "profile",
    active: pathname === "/profile",
  });

  return items;
}

function AuthenticatedPanelShell({
  children,
  pathname,
  user,
  onLogout,
}: AuthenticatedPanelShellProps) {
  const [isMobileNavigationOpen, setIsMobileNavigationOpen] = useState(false);

  const navigationItems = buildNavigationItems(pathname, user);

  const currentPageTitle = getCurrentPageTitle(pathname);

  function closeMobileNavigation() {
    setIsMobileNavigationOpen(false);
  }

  function openMobileNavigation() {
    setIsMobileNavigationOpen(true);
  }

  function handleLogout() {
    closeMobileNavigation();
    onLogout();
  }

  return (
    <div className={styles.panelShell}>
      <button
        className={[
          styles.panelShell__overlay,
          isMobileNavigationOpen ? styles.panelShell__overlayVisible : "",
        ]
          .filter(Boolean)
          .join(" ")}
        type="button"
        aria-label="Close navigation"
        onClick={closeMobileNavigation}
      />

      <aside
        className={[
          styles.panelShell__sidebar,
          isMobileNavigationOpen ? styles.panelShell__sidebarOpen : "",
        ]
          .filter(Boolean)
          .join(" ")}
        aria-label="Application navigation"
      >
        <header className={styles.panelShell__brandHeader}>
          <Link
            className={styles.panelShell__brand}
            href="/dashboard"
            onClick={closeMobileNavigation}
          >
            <span className={styles.panelShell__brandMark}>M</span>

            <span className={styles.panelShell__brandCopy}>
              <strong>Miletos</strong>

              <small>Workflow automation</small>
            </span>
          </Link>

          <button
            className={styles.panelShell__sidebarClose}
            type="button"
            aria-label="Close navigation"
            onClick={closeMobileNavigation}
          >
            <Icon name="x" size={20} />
          </button>
        </header>

        <p className={styles.panelShell__workspaceLabel}>Workspace</p>

        <nav className={styles.panelShell__navigation} aria-label="Primary navigation">
          {navigationItems.map((item) => (
            <Link
              key={item.href}
              className={[
                styles.panelShell__navigationLink,
                item.active ? styles.panelShell__navigationLinkActive : "",
              ]
                .filter(Boolean)
                .join(" ")}
              href={item.href}
              aria-current={item.active ? "page" : undefined}
              onClick={closeMobileNavigation}
            >
              <span className={styles.panelShell__navigationIcon}>
                <Icon name={item.icon} size={18} />
              </span>

              <span className={styles.panelShell__navigationCopy}>
                <strong>{item.label}</strong>

                <small>{item.description}</small>
              </span>
            </Link>
          ))}
        </nav>

        <div className={styles.panelShell__sidebarFooter}>
          <section className={styles.panelShell__security}>
            <span className={styles.panelShell__securityIcon}>
              <Icon name="shield" size={20} />
            </span>

            <div>
              <strong>Protected workspace</strong>

              <p>Session and permissions are resolved from your active account.</p>
            </div>
          </section>

          <Link
            className={styles.panelShell__userCard}
            href="/profile"
            onClick={closeMobileNavigation}
          >
            <PanelUserIdentity
              user={user}
              avatarClassName={styles.panelShell__userAvatar}
              identityClassName={styles.panelShell__userIdentity}
            />
          </Link>
        </div>
      </aside>

      <div className={styles.panelShell__workspace}>
        <header className={styles.panelShell__topbar}>
          <div className={styles.panelShell__topbarLeading}>
            <button
              className={styles.panelShell__menuButton}
              type="button"
              aria-label="Open navigation"
              aria-expanded={isMobileNavigationOpen}
              onClick={openMobileNavigation}
            >
              <Icon name="menu" size={22} />
            </button>

            <div className={styles.panelShell__pageContext}>
              <span>Miletos workspace</span>

              <strong>{currentPageTitle}</strong>
            </div>
          </div>

          <div className={styles.panelShell__topbarActions}>
            <Link
              className={styles.panelShell__topbarProfile}
              href="/profile"
              onClick={closeMobileNavigation}
            >
              <PanelUserIdentity
                user={user}
                avatarClassName={styles.panelShell__topbarAvatar}
                identityClassName={styles.panelShell__topbarIdentity}
              />
            </Link>

            <button
              className={styles.panelShell__logoutButton}
              type="button"
              onClick={handleLogout}
            >
              <Icon name="logout" size={19} />

              <span>Logout</span>
            </button>
          </div>
        </header>

        <div className={styles.panelShell__content}>{children}</div>
      </div>
    </div>
  );
}

export function PanelGuard({ children }: PanelGuardProps) {
  const pathname = usePathname();
  const router = useRouter();
  const logout = useLogout();

  const isClientReady = useSyncExternalStore(
    subscribeToClientReady,
    getClientSnapshot,
    getServerSnapshot,
  );

  const currentUserQuery = useCurrentUserQuery({
    enabled: isClientReady,
  });

  useEffect(() => {
    if (!isClientReady) {
      return;
    }

    if (!hasAccessToken()) {
      router.replace("/auth/login");
    }
  }, [isClientReady, router]);

  useEffect(() => {
    function handleUnauthorized() {
      logout();
      router.replace("/auth/login");
    }

    window.addEventListener(SESSION_UNAUTHORIZED_EVENT, handleUnauthorized);

    return () => {
      window.removeEventListener(SESSION_UNAUTHORIZED_EVENT, handleUnauthorized);
    };
  }, [logout, router]);

  useEffect(() => {
    if (!currentUserQuery.isError) {
      return;
    }

    if (currentUserQuery.error.isUnauthorized || currentUserQuery.error.isForbidden) {
      logout();
      router.replace("/auth/login");
    }
  }, [currentUserQuery.error, currentUserQuery.isError, logout, router]);

  function handleLogout() {
    logout();
    router.replace("/auth/login");
  }

  if (!isClientReady) {
    return (
      <main className={styles.panelGuard}>
        <section className={styles.panelGuard__card}>
          <p className={styles.panelGuard__eyebrow}>Miletos</p>

          <h1 className={styles.panelGuard__title}>Loading session</h1>

          <p className={styles.panelGuard__description}>
            We are preparing your authenticated session.
          </p>
        </section>
      </main>
    );
  }

  if (!hasAccessToken()) {
    return (
      <main className={styles.panelGuard}>
        <section className={styles.panelGuard__card}>
          <p className={styles.panelGuard__eyebrow}>Miletos</p>

          <h1 className={styles.panelGuard__title}>Redirecting</h1>

          <p className={styles.panelGuard__description}>
            You need to sign in before accessing this page.
          </p>
        </section>
      </main>
    );
  }

  if (currentUserQuery.isLoading || currentUserQuery.isPending) {
    return (
      <main className={styles.panelGuard}>
        <section className={styles.panelGuard__card}>
          <p className={styles.panelGuard__eyebrow}>Miletos</p>

          <h1 className={styles.panelGuard__title}>Loading session</h1>

          <p className={styles.panelGuard__description}>
            We are checking your authenticated session.
          </p>
        </section>
      </main>
    );
  }

  if (currentUserQuery.isError) {
    return (
      <main className={styles.panelGuard}>
        <section className={styles.panelGuard__card}>
          <p className={styles.panelGuard__eyebrow}>Miletos</p>

          <h1 className={styles.panelGuard__title}>Session error</h1>

          <p className={styles.panelGuard__description}>We could not verify your session.</p>

          <Box className={styles.panelGuard__error} role="alert">
            {currentUserQuery.error.message}
          </Box>
        </section>
      </main>
    );
  }

  if (!currentUserQuery.data) {
    return (
      <main className={styles.panelGuard}>
        <section className={styles.panelGuard__card}>
          <p className={styles.panelGuard__eyebrow}>Miletos</p>

          <h1 className={styles.panelGuard__title}>Account unavailable</h1>

          <p className={styles.panelGuard__description}>
            The authenticated account information could not be loaded.
          </p>
        </section>
      </main>
    );
  }

  return (
    <AuthenticatedPanelShell
      key={pathname}
      pathname={pathname}
      user={currentUserQuery.data}
      onLogout={handleLogout}
    >
      {children}
    </AuthenticatedPanelShell>
  );
}
