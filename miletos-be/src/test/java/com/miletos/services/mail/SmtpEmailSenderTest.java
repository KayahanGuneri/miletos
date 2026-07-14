package com.miletos.services.mail;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

import com.miletos.config.MailProperties;
import jakarta.mail.Session;
import jakarta.mail.internet.MimeMessage;
import java.util.Properties;
import java.io.ByteArrayOutputStream;
import java.nio.charset.StandardCharsets;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;
import org.springframework.mail.javamail.JavaMailSender;

class SmtpEmailSenderTest {
    @Test
    void sendsMultipartAlternativeHtmlAndTextMessage() throws Exception {
        JavaMailSender mailSender = mock(JavaMailSender.class);
        MimeMessage mimeMessage = new MimeMessage(Session.getInstance(new Properties()));
        when(mailSender.createMimeMessage()).thenReturn(mimeMessage);
        SmtpEmailSender sender = new SmtpEmailSender(
                mailSender, new MailProperties("noreply@miletos.local"));

        sender.send(new EmailMessage(
                "user@example.com", "Subject", "Text fallback", "<strong>HTML body</strong>"));

        ArgumentCaptor<MimeMessage> captor = ArgumentCaptor.forClass(MimeMessage.class);
        verify(mailSender).send(captor.capture());
        MimeMessage sent = captor.getValue();
        sent.saveChanges();
        assertThat(sent.getFrom()[0].toString()).isEqualTo("noreply@miletos.local");
        assertThat(sent.getAllRecipients()[0].toString()).isEqualTo("user@example.com");
        assertThat(sent.getSubject()).isEqualTo("Subject");
        assertThat(sent.getContentType()).startsWith("multipart/mixed");
        ByteArrayOutputStream rawMessage = new ByteArrayOutputStream();
        sent.writeTo(rawMessage);
        String mime = rawMessage.toString(StandardCharsets.UTF_8);
        assertThat(mime).contains("multipart/alternative");
        assertThat(mime).contains("Content-Type: text/plain; charset=UTF-8");
        assertThat(mime).contains("Content-Type: text/html;charset=UTF-8");
        assertThat(mime).contains("Text fallback");
        assertThat(mime).contains("<strong>HTML body</strong>");
    }
}
