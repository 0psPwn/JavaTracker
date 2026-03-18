package com.demo;

public class App {
    private final UserService userService = new UserService();

    public String run(String name) {
        return userService.load(name);
    }
}
