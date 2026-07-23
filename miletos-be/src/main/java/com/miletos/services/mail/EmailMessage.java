package com.miletos.services.mail;

public record EmailMessage(String to, String subject, String textBody, String htmlBody) {
}
