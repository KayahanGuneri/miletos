package com.miletos.services.mail;

import static org.assertj.core.api.Assertions.assertThat;

import java.util.Map;
import org.junit.jupiter.api.Test;

class MailTemplateRendererTest {

    private final MailTemplateRenderer mailTemplateRenderer = new MailTemplateRenderer();

    @Test
    void rendersInviteTemplate() {
        String renderedTemplate = mailTemplateRenderer.renderText(
                "invite-user.txt",
                Map.of(
                        "firstName", "Ada",
                        "inviteLink", "http://localhost:3000/complete-password?token=abc"
                )
        );

        assertThat(renderedTemplate).contains("Hello Ada,");
        assertThat(renderedTemplate).contains("You have been invited to Miletos.");
        assertThat(renderedTemplate).contains("http://localhost:3000/complete-password?token=abc");
        assertThat(renderedTemplate).doesNotContain("{{firstName}}");
        assertThat(renderedTemplate).doesNotContain("{{inviteLink}}");
    }

    @Test
    void rendersPasswordResetTemplate() {
        String renderedTemplate = mailTemplateRenderer.renderText(
                "password-reset.txt",
                Map.of(
                        "firstName", "Grace",
                        "resetLink", "http://localhost:3000/reset-password?token=xyz",
                        "expiresInMinutes", "30"
                )
        );

        assertThat(renderedTemplate).contains("Hello Grace,");
        assertThat(renderedTemplate).contains("A password reset was requested for your Miletos account.");
        assertThat(renderedTemplate).contains("http://localhost:3000/reset-password?token=xyz");
        assertThat(renderedTemplate).contains("This password reset link expires in 30 minutes.");
        assertThat(renderedTemplate).doesNotContain("{{firstName}}");
        assertThat(renderedTemplate).doesNotContain("{{resetLink}}");
        assertThat(renderedTemplate).doesNotContain("{{expiresInMinutes}}");
    }

    @Test
    void rendersEscapedHtmlWithVisibleFallbackLink() {
        String rendered = mailTemplateRenderer.renderHtml(
                "invite-user.html",
                Map.of("firstName", "<Ada & Co>", "inviteLink", "https://miletos.test/invite?a=1&b=2"));

        assertThat(rendered).contains("&lt;Ada &amp; Co&gt;");
        assertThat(rendered).contains("https://miletos.test/invite?a=1&amp;b=2");
        assertThat(rendered).doesNotContain("<Ada & Co>");
    }

    @Test
    void rendersEscapedPasswordResetHtmlWithVisibleFallbackLink() {
        String rendered = mailTemplateRenderer.renderHtml(
                "password-reset.html",
                Map.of(
                        "firstName", "<script>alert('name')</script>",
                        "resetLink", "https://miletos.test/reset?token=\"bad\"&next=<x>",
                        "expiresInMinutes", "30<script>alert(1)</script>"));

        assertThat(rendered).contains("&lt;script&gt;alert(&#39;name&#39;)&lt;/script&gt;");
        assertThat(rendered).contains("https://miletos.test/reset?token=&quot;bad&quot;&amp;next=&lt;x&gt;");
        assertThat(rendered).contains("30&lt;script&gt;alert(1)&lt;/script&gt; minutes");
        assertThat(rendered).doesNotContain("<script>");
    }
}
