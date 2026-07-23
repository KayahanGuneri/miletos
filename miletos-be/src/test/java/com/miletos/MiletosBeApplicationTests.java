package com.miletos;

import com.miletos.testsupport.PostgreSqlContainerSupport;
import org.junit.jupiter.api.Test;
import org.springframework.boot.test.context.SpringBootTest;

@SpringBootTest
class MiletosBeApplicationTests extends PostgreSqlContainerSupport {

    @Test
    void contextLoads() {
    }
}