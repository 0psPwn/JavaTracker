package com.example.demo;

public class BaseService {
    protected void audit(String value) {
        if (value == null) {
            return;
        }
    }
}
