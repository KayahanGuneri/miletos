# Miletos Backend

Miletos backend service.

## Stack

- Java 21
- Spring Boot 3.5.x
- Spring MVC
- Spring Data JPA
- PostgreSQL
- Flyway
- Spring Security
- Bean Validation
- Maven

## Architecture Rules

- CRUD operations are implemented in Java backend.
- Controllers receive request DTOs.
- Request DTOs must not leak into application services.
- Presentation mappers convert request DTOs into application commands or domain input models.
- Services work with commands, domain models, repositories and ports.
- Services must not depend on web/request-layer DTOs.
- Dependency Inversion must be protected: application layer depends on ports/interfaces, infrastructure implements them.
- Workflow execution engine will be handled separately with Go in later phases.
- Saga pattern is not part of this project scope.

## Package Layout

- presentation: controllers, request/response DTOs, mappers
- application: use cases, services, ports
- domain: entities, value objects, domain rules
- infrastructure: JPA repositories, adapters, persistence configuration

## Local Commands

mvn clean package -DskipTests

Remote repository is intentionally not configured.
