package com.example.demo;

public class UserService extends BaseService implements ProfileProvider {
    private final UserRepository repository = new UserRepository();

    public String loadProfile(String userId) {
        String sql = buildLookupSql(userId);
        return repository.query(sql);
    }

    private String buildLookupSql(String userId) {
        return "select * from users where id = '" + userId + "'";
    }
}
