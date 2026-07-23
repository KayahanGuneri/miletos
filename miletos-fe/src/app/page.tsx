import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Box } from "@/components/lib/box/Box";

export default function Home() {
  return (
    <main className="home-page">
      <section className="home-page__hero">
        <Box className="home-page__content">
          <p className="home-page__eyebrow">Miletos Workflow Automation Platform</p>
          <h1 className="home-page__title">Node-based workflow management starts here.</h1>
          <p className="home-page__description">
            Miletos will provide company-based user management, workflow CRUD screens, responsive
            dashboards, and integration points for a Go-based workflow engine.
          </p>
          <Box className="home-page__actions">
            <Button>Start foundation</Button>
            <Button variant={ButtonVariant.Secondary}>View architecture</Button>
          </Box>
        </Box>
      </section>
    </main>
  );
}
