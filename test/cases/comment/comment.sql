-- test create table
drop table if exists t1;
create table t1(
col1 int comment '这是第一列',
col2 float comment '"%$^&*()_+@!',
col3 varchar comment 'KNcR5P2ksILgsZD5lTndyuEzw49gxR2RlfU7nkNhAFOKIhig6roVYgS6yQDBkuzH790peOVKgTKUasKxuepzKqsYqQg3gDtgn0KEkC1TGVh2RU6QcdQolDbnwXsnst4gVCsF1RPp975efCff8gtXKgUtRVPdSM41vtgvKkChcUIaHU9UuLvoy6BhSm9g60VKd8NTxWRiYlzdhGiTTwtqVOq7wE9NHZt8Xq55Tz9PogoGGsgObH5llIcRAQkZUraZcYBwRoHHISouTm5whECuV8X84I0s8gx1DrQulbNCQuPVUsaAFFrsawonlvLKAOYgFPg1CheDMg63wwvY7sg1W8uCNu0ZzwmRlltC8BK1y5L1E690OV84bqNbFlkInxgl9W9CsgbIwKrFXoShkfB6DnBN5khLhH4oafYkTMWh71ZEc70t583PAxZFGNEGCALP482teY4z4Vc18EkKnG5lRg4WPNVgR5lkpFkVxvwtD2GgGvVmwdwgcYG5OlSSQGhOLDDM9sIqOlN5eyIG1kcZB8tmpMicgg7IbaKxW0ACt1OlQiLufPCvsSXlU3STSBfr3HPcbZyIGfMqkxFvpGoCHDB1D0fPlLNWIksGPOXGa6ZuXrpgdqNhFgbWTUgSllMnm59RwTZgazXXLitNVgLYK9zlVv8k6T6N0orPot2V7BvLLvxNzEvfTytliQAy418XHMb3fyR5ko34lia7hZXEqsOuEq0iTgIyHBvYn1iD3wlcnu29UTB1267O8dgL03nMmWHPFqEudVMlxeEoabRSGm2LxIlRYN8peOFBvper4Iomg7qAEaodHU1SctIGuGVTuKK5K6d2rfWs8tEokxbolTG4gxMcVzSgrcvv01eNEfCWgEYXdNShX56Wqods5qgRXNn0EeMTHyBP4tZsr6LGNgqKibYemO58VL4SE5GnGURSGW0AmFg27m1zy9qucbgAyGgDmTYGRBkxDIgSNUbVyRNe1u8RYXpWaaSGg4YwEzcgXzUKSBNS0KPXMI8GzbVM',
col4 bool comment ''
);

-- @bvt:issue#4720
show create table t1;
-- @bvt:issue
drop table t1;

drop table if exists t2;
-- echo Comment for field 'col3' is too long (max = 1024)
create table t2(
col1 int comment '这是第一列',
col2 float comment '"%$^&*()_+@!"',
col3 varchar comment 'KNcR5P2ksILgsZD5lTndyuEzw49gxR2RlfU7nkNhAFOKIhig6roVYgS6yQDBkuzH790peOVKgTKUasKxuepzKqsYqQg3gDtgn0KEkC1TGVh2RU6QcdQolDbnwXsnst4gVCsF1RPp975efCff8gtXKgUtRVPdSM41vtgvKkChcUIaHU9UuLvoy6BhSm9g60VKd8NTxWRiYlzdhGiTTwtqVOq7wE9NHZt8Xq55Tz9PogoGGsgObH5llIcRAQkZUraZcYBwRoHHISouTm5whECuV8X84I0s8gx1DrQulbNCQuPVUsaAFFrsawonlvLKAOYgFPg1CheDMg63wwvY7sg1W8uCNu0ZzwmRlltC8BK1y5L1E690OV84bqNbFlkInxgl9W9CsgbIwKrFXoShkfB6DnBN5khLhH4oafYkTMW3h71ZEc70t583PAxZFGNEGCALP482teY4z4Vc18EkKnG5lRg4WPNVgR5lkpFkVxvwtD2GgGvVmwdwgcYG5OlSSQGhOLDDM9sIqOlN5eyIG1kcZB8tmpMicgg7IbaKxW0ACt1OlQiLufPCvsSXlU3STSBfr3HPcbZyIGfMqkxFvpGoCHDB1D0fPlLNWIksGPOXGa6ZuXrpgdqNhFgbWTUgSllMnm59RwTZgazXXLitNVgLYK9zlVv8k6T6N0orPot2V7BvLLvxNzEvfTytliQAy418XHMb3fyR5ko34lia7hZXEqsOuEq0iTgIyHBvYn1iD3wlcnu29UTB1267O8dgL03nMmWHPFqEudVMlxeEoabRSGm2LxIlRYN8peOFBvper4Iomg7qAEaodHU1SctIGuGVTuKK5K6d2rfWs8tEokxbolTG4gxMcVzSgrcvv01eNEfCWgEYXdNShX56Wqods5qgRXNn0EeMTHyBP4tZsr6LGNgqKibYemO58VL4SE5GnGURSGW0AmFg27m1zy9qucbgAyGgDmTYGRBkxDIgSNUbVyRNe1u8RYXpWaaSGg4YwEzcgXzUKSBNS0KPXMI8GzbVM',
col4 bool comment ''
);

-- @bvt:issue#4720
show create table t2;
-- @bvt:issue
drop table t2;

drop table if exists t3;
create table t3(
col1 int comment '"这是第一列"/',
col2 float comment '"%$^&*()_+@!"',
col3 varchar comment 'KNcR5P2ksILgsZD5lTndyuEzw49gxR2RlU7nkNhAFOKIhig6roVYgS6yQDBkuzH790peOVKgTKUasKxuepzKqsYqQg3gDtgn0KEkC1TGVh2RU6QcdQolDbnwXsnst4gVCsF1RPp975efCff8gtXKgUtRVPdSM41vtgvKkChcUIaHU9UuLvoy6BhSm9g60VKd8NTxWRiYlzdhGiTTwtqVOq7wE9NHZt8Xq55Tz9PogoGGsgObH5llIcRAQkZUraZcYBwRoHHISouTm5whECuV8X84I0s8gx1DrQulbNCQuPVUsaAFFrsawonlvLKAOYgFPg1CheDMg63wwvY7sg1W8uCNu0ZzwmRlltC8BK1y5L1E690OV84bqNbFlkInxgl9W9CsgbIwKrFXoShkfB6DnBN5khLhH4oafYkTMW3h71ZEc70t583PAxZFGNEGCALP482teY4z4Vc18EkKnG5lRg4WPNVgR5lkpFkVxvwtD2GgGvVmwdwgcYG5OlSSQGhOLDDM9sIqOlN5eyIG1kcZB8tmpMicgg7IbaKxW0ACt1OlQiLufPCvsSXlU3STSBfr3HPcbZyIGfMqkxFvpGoCHDB1D0fPlLNWIksGPOXGa6ZuXrpgdqNhFgbWTUgSllMnm59RwTZgazXXLitNVgLYK9zlVv8k6T6N0orPot2V7BvLLvxNzEvfTytliQAy418XHMb3fyR5ko34lia7hZXEqsOuEq0iTgIyHBvYn1iD3wlcnu29UTB1267O8dgL03nMmWHPFqEudVMlxeEoabRSGm2LxIlRYN8peOFBvper4Iomg7qAEaodHU1SctIGuGVTuKK5K6d2rfWs8tEokxbolTG4gxMcVzSgrcvv01eNEfCWgEYXdNShX56Wqods5qgRXNn0EeMTHyBP4tZsr6LGNgqKibYemO58VL4SE5GnGURSGW0AmFg27m1zy9qucbgAyGgDmTYGRBkxDIgSNUbVyRNe1u8RYXpWaaSGg4YwEzcgXzUKSBNS0KPXMI8GzbVM',
col4 bool comment ''
) comment '这是一个t3表';

-- @bvt:issue#4720
show create table t3;
-- @bvt:issue
drop table t3;


drop table if exists t4;
create table t4(
col1 int comment '"这是第一列"/',
col2 float comment '"%$^&*()_+@!"',
col3 varchar comment 'KNcR5P2ksILgsZD5lTndyuEzw49gxR2RlU7nkNhAFOKIhig6roVYgS6yQDBkuzH790peOVKgTKUasKxuepzKqsYqQg3gDtgn0KEkC1TGVh2RU6QcdQolDbnwXsnst4gVCsF1RPp975efCff8gtXKgUtRVPdSM41vtgvKkChcUIaHU9UuLvoy6BhSm9g60VKd8NTxWRiYlzdhGiTTwtqVOq7wE9NHZt8Xq55Tz9PogoGGsgObH5llIcRAQkZUraZcYBwRoHHISouTm5whECuV8X84I0s8gx1DrQulbNCQuPVUsaAFFrsawonlvLKAOYgFPg1CheDMg63wwvY7sg1W8uCNu0ZzwmRlltC8BK1y5L1E690OV84bqNbFlkInxgl9W9CsgbIwKrFXoShkfB6DnBN5khLhH4oafYkTMW3h71ZEc70t583PAxZFGNEGCALP482teY4z4Vc18EkKnG5lRg4WPNVgR5lkpFkVxvwtD2GgGvVmwdwgcYG5OlSSQGhOLDDM9sIqOlN5eyIG1kcZB8tmpMicgg7IbaKxW0ACt1OlQiLufPCvsSXlU3STSBfr3HPcbZyIGfMqkxFvpGoCHDB1D0fPlLNWIksGPOXGa6ZuXrpgdqNhFgbWTUgSllMnm59RwTZgazXXLitNVgLYK9zlVv8k6T6N0orPot2V7BvLLvxNzEvfTytliQAy418XHMb3fyR5ko34lia7hZXEqsOuEq0iTgIyHBvYn1iD3wlcnu29UTB1267O8dgL03nMmWHPFqEudVMlxeEoabRSGm2LxIlRYN8peOFBvper4Iomg7qAEaodHU1SctIGuGVTuKK5K6d2rfWs8tEokxbolTG4gxMcVzSgrcvv01eNEfCWgEYXdNShX56Wqods5qgRXNn0EeMTHyBP4tZsr6LGNgqKibYemO58VL4SE5GnGURSGW0AmFg27m1zy9qucbgAyGgDmTYGRBkxDIgSNUbVyRNe1u8RYXpWaaSGg4YwEzcgXzUKSBNS0KPXMI8GzbVM',
col4 bool comment ''
) comment '这是一个t4表';

-- @bvt:issue#4720
show create table t4;
-- @bvt:issue
drop table t4;


drop table if exists t5;
create table t5(
col1 int comment '"这是第一列"/',
col2 float comment '"%$^&*()_+@!"',
col3 varchar comment 'KNcR5P2ksILgsZD5lTndyuEzw49gxR2RlU7nkNhAFOKIhig6roVYgS6yQDBkuzH790peOVKgTKUasKxuepzKqsYqQg3gDtgn0KEkC1TGVh2RU6QcdQolDbnwXsnst5gVCsF1RPp975efCff8gtXKgUtRVPdSM41vtgvKkChcUIaHU9UuLvoy6BhSm9g60VKd8NTxWRiYlzdhGiTTwtqVOq7wE9NHZt8Xq55Tz9PogoGGsgObH5llIcRAQkZUraZcYBwRoHHISouTm5whECuV8X84I0s8gx1DrQulbNCQuPVUsaAFFrsawonlvLKAOYgFPg1CheDMg63wwvY7sg1W8uCNu0ZzwmRlltC8BK1y5L1E690OV84bqNbFlkInxgl9W9CsgbIwKrFXoShkfB6DnBN5khLhH4oafYkTMW3h71ZEc70t583PAxZFGNEGCALP482teY4z4Vc18EkKnG5lRg4WPNVgR5lkpFkVxvwtD2GgGvVmwdwgcYG5OlSSQGhOLDDM9sIqOlN5eyIG1kcZB8tmpMicgg7IbaKxW0ACt1OlQiLufPCvsSXlU3STSBfr3HPcbZyIGfMqkxFvpGoCHDB1D0fPlLNWIksGPOXGa6ZuXrpgdqNhFgbWTUgSllMnm59RwTZgazXXLitNVgLYK9zlVv8k6T6N0orPot2V7BvLLvxNzEvfTytliQAy418XHMb3fyR5ko34lia7hZXEqsOuEq0iTgIyHBvYn1iD3wlcnu29UTB1267O8dgL03nMmWHPFqEudVMlxeEoabRSGm2LxIlRYN8peOFBvper4Iomg7qAEaodHU1SctIGuGVTuKK5K6d2rfWs8tEokxbolTG4gxMcVzSgrcvv01eNEfCWgEYXdNShX56Wqods5qgRXNn0EeMTHyBP4tZsr6LGNgqKibYemO58VL4SE5GnGURSGW0AmFg27m1zy9qucbgAyGgDmTYGRBkxDIgSNUbVyRNe1u8RYXpWaaSGg4YwEzcgXzUKSBNS0KPXMI8GzbVM',
col4 bool comment ''
) comment 'KNcR5P2ksILgsZD5lTndyuEzw49gxR2RlU7nkNhAFOKIhig6roVYgS6yQDBkuzH790peOVKgTKUasKxuepzKqsYqQg3gDtgn0KEkC1TGVh2RU6QcdQolDbnwXsnst5gVCsF1RPp975efCff8gtXKgUtRVPdSM41vtgvKkChcUIaHU9UuLvoy6BhSm9g60VKd8NTxWRiYlzdhGiTTwtqVOq7wE9NHZt8Xq55Tz9PogoGGsgObH5llIcRAQkZUraZcYBwRoHHISouTm5whECuV8X84I0s8gx1DrQulbNCQuPVUsaAFFrsawonlvLKAOYgFPg1CheDMg63wwvY7sg1W8uCNu0ZzwmRlltC8BK1y5L1E690OV84bqNbFlkInxgl9W9CsgbIwKrFXoShkfB6DnBN5khLhH4oafYkTMW3h71ZEc70t583PAxZFGNEGCALP482teY4z4Vc18EkKnG5lRg4WPNVgR5lkpFkVxvwtD2GgGvVmwdwgcYG5OlSSQGhOLDDM9sIqOlN5eyIG1kcZB8tmpMicgg7IbaKxW0ACt1OlQiLufPCvsSXlU3STSBfr3HPcbZyIGfMqkxFvpGoCHDB1D0fPlLNWIksGPOXGa6ZuXrpgdqNhFgbWTUgSllMnm59RwTZgazXXLitNVgLYK9zlVv8k6T6N0orPot2V7BvLLvxNzEvfTytliQAy418XHMb3fyR5ko34lia7hZXEqsOuEq0iTgIyHBvYn1iD3wlcnu29UTB1267O8dgL03nMmWHPFqEudVMlxeEoabRSGm2LxIlRYN8peOFBvper4Iomg7qAEaodHU1SctIGuGVTuKK5K6d2rfWs8tEokxbolTG4gxMcVzSgrcvv01eNEfCWgEYXdNShX56Wqods5qgRXNn0EeMTHyBP4tZsr6LGNgqKibYemO58VL4SE5GnGURSGW0AmFg27m1zy9qucbgAyGgDmTYGRBkxDIgSNUbVyRNe1u8RYXpWaaSGg4YwEzcgXzUKSBNS0KPXMI8GzbVM';

-- @bvt:issue#4720
show create table t5;
-- @bvt:issue
drop table t5;

drop table if exists t6;
create table t6(
col1 int comment '"这是第一列"/',
col2 float comment '"%$^&*()_+@!"',
col3 varchar comment 'KNcR5P2ksILgsZD5lTndyuEzw49gxR2RlU7nkNhAFOKIhig6roVYgS6yQDBkuzH790peOVKgTKUasKxuepzKqsYqQg3gDtgn0KEkC1TGVh2RU6QcdQolDbnwXsnst6gVCsF1RPp975efCff8gtXKgUtRVPdSM41vtgvKkChcUIaHU9UuLvoy6BhSm9g60VKd8NTxWRiYlzdhGiTTwtqVOq7wE9NHZt8Xq55Tz9PogoGGsgObH5llIcRAQkZUraZcYBwRoHHISouTm5whECuV8X84I0s8gx1DrQulbNCQuPVUsaAFFrsawonlvLKAOYgFPg1CheDMg63wwvY7sg1W8uCNu0ZzwmRlltC8BK1y5L1E690OV84bqNbFlkInxgl9W9CsgbIwKrFXoShkfB6DnBN5khLhH4oafYkTMW3h71ZEc70t683PAxZFGNEGCALP482teY4z4Vc18EkKnG5lRg4WPNVgR5lkpFkVxvwtD2GgGvVmwdwgcYG5OlSSQGhOLDDM9sIqOlN5eyIG1kcZB8tmpMicgg7IbaKxW0ACt1OlQiLufPCvsSXlU3STSBfr3HPcbZyIGfMqkxFvpGoCHDB1D0fPlLNWIksGPOXGa6ZuXrpgdqNhFgbWTUgSllMnm59RwTZgazXXLitNVgLYK9zlVv8k6T6N0orPot2V7BvLLvxNzEvfTytliQAy418XHMb3fyR5ko34lia7hZXEqsOuEq0iTgIyHBvYn1iD3wlcnu29UTB1267O8dgL03nMmWHPFqEudVMlxeEoabRSGm2LxIlRYN8peOFBvper4Iomg7qAEaodHU1SctIGuGVTuKK5K6d2rfWs8tEokxbolTG4gxMcVzSgrcvv01eNEfCWgEYXdNShX56Wqods5qgRXNn0EeMTHyBP4tZsr6LGNgqKibYemO58VL4SE5GnGURSGW0AmFg27m1zy9qucbgAyGgDmTYGRBkxDIgSNUbVyRNe1u8RYXpWaaSGg4YwEzcgXzUKSBNS0KPXMI8GzbVM',
col4 bool comment ''
) comment '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';

-- @bvt:issue#4720
show create table t6;
-- @bvt:issue
drop table t6;


drop table if exists t7;
-- Comment for table 't7' is too long (max = 2048)
create table t7(
col1 int comment '"这是第一列"/',
col2 float comment '"%$^&*()_+@!"',
col3 varchar comment 'KNcR5P2ksILgsZD5lTndyuEzw49gxR2RlU7nkNhAFOKIhig6roVYgS6yQDBkuzH790peOVKgTKUasKxuepzKqsYqQg3gDtgn0KEkC1TGVh2RU6QcdQolDbnwXsnst6gVCsF1RPp975efCff8gtXKgUtRVPdSM41vtgvKkChcUIaHU9UuLvoy6BhSm9g60VKd8NTxWRiYlzdhGiTTwtqVOq7wE9NHZt8Xq55Tz9PogoGGsgObH5llIcRAQkZUraZcYBwRoHHISouTm5whECuV8X84I0s8gx1DrQulbNCQuPVUsaAFFrsawonlvLKAOYgFPg1CheDMg63wwvY7sg1W8uCNu0ZzwmRlltC8BK1y5L1E690OV84bqNbFlkInxgl9W9CsgbIwKrFXoShkfB6DnBN5khLhH4oafYkTMW3h71ZEc70t683PAxZFGNEGCALP482teY4z4Vc18EkKnG5lRg4WPNVgR5lkpFkVxvwtD2GgGvVmwdwgcYG5OlSSQGhOLDDM9sIqOlN5eyIG1kcZB8tmpMicgg7IbaKxW0ACt1OlQiLufPCvsSXlU3STSBfr3HPcbZyIGfMqkxFvpGoCHDB1D0fPlLNWIksGPOXGa6ZuXrpgdqNhFgbWTUgSllMnm59RwTZgazXXLitNVgLYK9zlVv8k6T6N0orPot2V7BvLLvxNzEvfTytliQAy418XHMb3fyR5ko34lia7hZXEqsOuEq0iTgIyHBvYn1iD3wlcnu29UTB1267O8dgL03nMmWHPFqEudVMlxeEoabRSGm2LxIlRYN8peOFBvper4Iomg7qAEaodHU1SctIGuGVTuKK5K6d2rfWs8tEokxbolTG4gxMcVzSgrcvv01eNEfCWgEYXdNShX56Wqods5qgRXNn0EeMTHyBP4tZsr6LGNgqKibYemO58VL4SE5GnGURSGW0AmFg27m1zy9qucbgAyGgDmTYGRBkxDIgSNUbVyRNe1u8RYXpWaaSGg4YwEzcgXzUKSBNS0KPXMI8GzbVM',
col4 bool comment ''
) comment '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';

-- @bvt:issue#4720
show create table t7;
-- @bvt:issue
drop table t7;


drop table if exists t8;
create table t8(
col1 int comment '这是第一列',
col2 float comment '"%$^&*()_+@!\'',
col3 varchar comment 'KNcR5P2ksILgsZD5lTndyuEzw49gxR2RlfU7nkNhAFOKIhig6roVYgS6yQDBkuzH790peOVKgTKUasKxuepzKqsYqQg3gDtgn0KEkC1TGVh2RU6QcdQolDbnwXsnst4gVCsF1RPp975efCff8gtXKgUtRVPdSM41vtgvKkChcUIaHU9UuLvoy6BhSm9g60VKd8NTxWRiYlzdhGiTTwtqVOq7wE9NHZt8Xq55Tz9PogoGGsgObH5llIcRAQkZUraZcYBwRoHHISouTm5whECuV8X84I0s8gx1DrQulbNCQuPVUsaAFFrsawonlvLKAOYgFPg1CheDMg63wwvY7sg1W8uCNu0ZzwmRlltC8BK1y5L1E690OV84bqNbFlkInxgl9W9CsgbIwKrFXoShkfB6DnBN5khLhH4oafYkTMWh71ZEc70t583PAxZFGNEGCALP482teY4z4Vc18EkKnG5lRg4WPNVgR5lkpFkVxvwtD2GgGvVmwdwgcYG5OlSSQGhOLDDM9sIqOlN5eyIG1kcZB8tmpMicgg7IbaKxW0ACt8OlQiLufPCvsSXlU3STSBfr3HPcbZyIGfMqkxFvpGoCHDB1D0fPlLNWIksGPOXGa6ZuXrpgdqNhFgbWTUgSllMnm59RwTZgazXXLitNVgLYK9zlVv8k6T6N0orPot2V7BvLLvxNzEvfTytliQAy418XHMb3fyR5ko34lia7hZXEqsOuEq0iTgIyHBvYn1iD3wlcnu29UTB1267O8dgL03nMmWHPFqEudVMlxeEoabRSGm2LxIlRYN8peOFBvper4Iomg7qAEaodHU1SctIGuGVTuKK5K6d2rfWs8tEokxbolTG4gxMcVzSgrcvv01eNEfCWgEYXdNShX56Wqods5qgRXNn0EeMTHyBP4tZsr6LGNgqKibYemO58VL4SE5GnGURSGW0AmFg27m1zy9qucbgAyGgDmTYGRBkxDIgSNUbVyRNe1u8RYXpWaaSGg4YwEzcgXzUKSBNS0KPXMI8GzbVM',
col4 bool comment ''
);

-- @bvt:issue#4720
show create table t8;
-- @bvt:issue
drop table t8;
