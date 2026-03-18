package com.example.demo;

public class UserController {
    private final UserService userService = new UserService();

    public String getProfile(String userId) {
        String normalized = userId.trim();
        return userService.loadProfile(normalized);
    }
}
