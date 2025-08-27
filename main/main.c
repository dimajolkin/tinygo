#include <stdio.h>
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "driver/gpio.h"
#include "esp_log.h"

// GPIO pin for LED blinking
#define BLINK_GPIO GPIO_NUM_4

static const char *TAG = "blink";

void app_main(void)
{
    ESP_LOGI(TAG, "ESP32-S3 Native Blink Example Started!");
    ESP_LOGI(TAG, "Blinking GPIO%d every 1 second", BLINK_GPIO);

    // Configure GPIO
    gpio_reset_pin(BLINK_GPIO);
    gpio_set_direction(BLINK_GPIO, GPIO_MODE_OUTPUT);

    while (1) {
        ESP_LOGI(TAG, "Turning the LED ON");
        gpio_set_level(BLINK_GPIO, 1);
        vTaskDelay(1000 / portTICK_PERIOD_MS);
        
        ESP_LOGI(TAG, "Turning the LED OFF");
        gpio_set_level(BLINK_GPIO, 0);
        vTaskDelay(1000 / portTICK_PERIOD_MS);
    }
}
