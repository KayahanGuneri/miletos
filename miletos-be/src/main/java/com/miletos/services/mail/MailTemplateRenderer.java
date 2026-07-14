package com.miletos.services.mail;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.Map;

import org.springframework.core.io.ClassPathResource;
import org.springframework.stereotype.Service;
import org.springframework.util.StreamUtils;

@Service
public class MailTemplateRenderer {
    private static final String TEMPLATE_ROOT = "mail-templates/";

    public String renderText(String templateName, Map<String, String> variables) {
        return render(templateName, variables, false);
    }

    public String renderHtml(String templateName, Map<String, String> variables) {
        return render(templateName, variables, true);
    }

    private String render(String templateName, Map<String, String> variables, boolean escapeHtml) {
        String rendered = loadTemplate(templateName);
        for (Map.Entry<String, String> variable : variables.entrySet()) {
            String value = escapeHtml ? escapeHtml(variable.getValue()) : variable.getValue();
            rendered = rendered.replace("{{" + variable.getKey() + "}}", value);
        }
        return rendered;
    }

    private String escapeHtml(String value) {
        return value.replace("&", "&amp;").replace("<", "&lt;")
                .replace(">", "&gt;").replace("\"", "&quot;").replace("'", "&#39;");
    }

    private String loadTemplate(String templateName) {
        try {
            return StreamUtils.copyToString(
                    new ClassPathResource(TEMPLATE_ROOT + templateName).getInputStream(),
                    StandardCharsets.UTF_8);
        } catch (IOException exception) {
            throw new IllegalStateException("Could not load mail template: " + templateName, exception);
        }
    }
}
