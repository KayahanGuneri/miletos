package com.miletos.common.exception;

import com.miletos.common.response.ApiErrorResponse;
import com.miletos.common.response.ApiErrorResponseFactory;
import jakarta.servlet.http.HttpServletRequest;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Set;
import org.springframework.http.HttpMethod;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.http.converter.HttpMessageNotReadableException;
import org.springframework.validation.FieldError;
import org.springframework.web.HttpMediaTypeNotSupportedException;
import org.springframework.web.HttpRequestMethodNotSupportedException;
import org.springframework.web.bind.MethodArgumentNotValidException;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.RestControllerAdvice;
import org.springframework.web.method.annotation.MethodArgumentTypeMismatchException;
import org.springframework.web.multipart.MaxUploadSizeExceededException;
import org.springframework.web.multipart.support.MissingServletRequestPartException;

@RestControllerAdvice
public class GlobalExceptionHandler {

  private final ApiErrorResponseFactory apiErrorResponseFactory;

  public GlobalExceptionHandler(ApiErrorResponseFactory apiErrorResponseFactory) {
    this.apiErrorResponseFactory = apiErrorResponseFactory;
  }

  @ExceptionHandler(MiletosException.class)
  public ResponseEntity<ApiErrorResponse> handleMiletosException(
      MiletosException exception, HttpServletRequest request) {
    ApiErrorResponse response =
        apiErrorResponseFactory.create(
            exception.getCode(),
            exception.getCode().name(),
            request.getRequestURI(),
            exception.getFieldErrors());

    return ResponseEntity.status(exception.getHttpStatus()).body(response);
  }

  @ExceptionHandler(MethodArgumentNotValidException.class)
  public ResponseEntity<ApiErrorResponse> handleValidationException(
      MethodArgumentNotValidException exception, HttpServletRequest request) {
    Map<String, String> fieldErrors = new LinkedHashMap<>();

    exception
        .getBindingResult()
        .getFieldErrors()
        .forEach(
            fieldError ->
                fieldErrors.put(fieldError.getField(), resolveValidationCode(fieldError)));

    ApiErrorResponse response =
        apiErrorResponseFactory.create(
            ErrorCode.VALIDATION_FAILED,
            ErrorCode.VALIDATION_FAILED.name(),
            request.getRequestURI(),
            fieldErrors);

    return ResponseEntity.badRequest().body(response);
  }

  @ExceptionHandler(HttpMessageNotReadableException.class)
  public ResponseEntity<ApiErrorResponse> handleMalformedRequest(
      HttpMessageNotReadableException exception, HttpServletRequest request) {
    return ResponseEntity.badRequest().body(createResponse(ErrorCode.MALFORMED_REQUEST, request));
  }

  @ExceptionHandler(HttpMediaTypeNotSupportedException.class)
  public ResponseEntity<ApiErrorResponse> handleUnsupportedMediaType(
      HttpMediaTypeNotSupportedException exception, HttpServletRequest request) {
    return ResponseEntity.status(HttpStatus.UNSUPPORTED_MEDIA_TYPE)
        .body(createResponse(ErrorCode.UNSUPPORTED_MEDIA_TYPE, request));
  }

  @ExceptionHandler(HttpRequestMethodNotSupportedException.class)
  public ResponseEntity<ApiErrorResponse> handleMethodNotSupported(
      HttpRequestMethodNotSupportedException exception, HttpServletRequest request) {
    ResponseEntity.BodyBuilder builder = ResponseEntity.status(HttpStatus.METHOD_NOT_ALLOWED);

    Set<HttpMethod> allowedMethods = exception.getSupportedHttpMethods();

    if (allowedMethods != null && !allowedMethods.isEmpty()) {
      builder.allow(allowedMethods.toArray(HttpMethod[]::new));
    }

    return builder.body(createResponse(ErrorCode.METHOD_NOT_ALLOWED, request));
  }

  @ExceptionHandler(MethodArgumentTypeMismatchException.class)
  public ResponseEntity<ApiErrorResponse> handleTypeMismatch(
      MethodArgumentTypeMismatchException exception, HttpServletRequest request) {
    Map<String, String> fieldErrors = Map.of(exception.getName(), "INVALID_VALUE");

    ApiErrorResponse response =
        apiErrorResponseFactory.create(
            ErrorCode.VALIDATION_FAILED,
            ErrorCode.VALIDATION_FAILED.name(),
            request.getRequestURI(),
            fieldErrors);

    return ResponseEntity.badRequest().body(response);
  }

  @ExceptionHandler(MissingServletRequestPartException.class)
  public ResponseEntity<ApiErrorResponse> handleMissingRequestPart(
      MissingServletRequestPartException exception, HttpServletRequest request) {
    Map<String, String> fieldErrors =
        Map.of(exception.getRequestPartName(), ErrorCode.MISSING_REQUEST_PART.name());

    ApiErrorResponse response =
        apiErrorResponseFactory.create(
            ErrorCode.MISSING_REQUEST_PART,
            ErrorCode.MISSING_REQUEST_PART.name(),
            request.getRequestURI(),
            fieldErrors);

    return ResponseEntity.badRequest().body(response);
  }

  @ExceptionHandler(MaxUploadSizeExceededException.class)
  public ResponseEntity<ApiErrorResponse> handleMaxUploadSizeExceeded(
      MaxUploadSizeExceededException exception, HttpServletRequest request) {
    return ResponseEntity.status(HttpStatus.PAYLOAD_TOO_LARGE)
        .body(createResponse(ErrorCode.PROFILE_PHOTO_TOO_LARGE, request));
  }

  @ExceptionHandler(Exception.class)
  public ResponseEntity<ApiErrorResponse> handleUnexpectedException(
      Exception exception, HttpServletRequest request) {
    return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR)
        .body(createResponse(ErrorCode.INTERNAL_SERVER_ERROR, request));
  }

  private ApiErrorResponse createResponse(ErrorCode code, HttpServletRequest request) {
    return apiErrorResponseFactory.create(code, code.name(), request.getRequestURI());
  }

  private String resolveValidationCode(FieldError fieldError) {
    String validationCode = fieldError.getCode();

    if (validationCode == null) {
      return "INVALID_VALUE";
    }

    return switch (validationCode) {
      case "NotBlank", "NotNull", "NotEmpty" -> "REQUIRED";
      case "Email" -> "INVALID_EMAIL";
      case "Size" -> "INVALID_SIZE";
      case "Pattern" -> "INVALID_FORMAT";
      default -> "INVALID_VALUE";
    };
  }
}
