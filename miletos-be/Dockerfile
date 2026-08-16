FROM maven:3.9-eclipse-temurin-21 AS build

WORKDIR /workspace
COPY pom.xml ./
COPY contracts ./contracts
COPY src ./src
RUN mvn -B -DskipTests package

FROM eclipse-temurin:21-jre-alpine

RUN addgroup -S miletos && adduser -S -G miletos miletos \
    && mkdir -p /app/uploads/profile-photos /app/workflow-inputs \
    && chown -R miletos:miletos /app
WORKDIR /app
COPY --from=build --chown=miletos:miletos /workspace/target/miletos-be-0.0.1-SNAPSHOT.jar /app/miletos-be.jar

USER miletos
EXPOSE 8080
ENTRYPOINT ["java", "-jar", "/app/miletos-be.jar"]
